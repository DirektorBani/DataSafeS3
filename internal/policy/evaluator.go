package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Document is a minimal bucket policy subset.
type Document struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement is one IAM policy statement. Action, Resource and Condition values
// accept either a string or an array.
type Statement struct {
	Sid       string                    `json:"Sid,omitempty"`
	Effect    string                    `json:"Effect"`
	Principal json.RawMessage           `json:"Principal,omitempty"`
	Action    StringList                `json:"Action"`
	Resource  StringList                `json:"Resource"`
	Condition map[string]map[string]any `json:"Condition,omitempty"`
}

// StringList unmarshals a JSON string or array of strings.
type StringList []string

func (s *StringList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// EvalContext is the request facts policy evaluation may use.
type EvalContext struct {
	Action          string
	Resource        string
	Principals      []string
	Anonymous       bool
	SourceIP        string
	SecureTransport bool
}

func BucketARN(bucket string) string {
	return "arn:aws:s3:::" + bucket
}

func ObjectARN(bucket, key string) string {
	return "arn:aws:s3:::" + bucket + "/" + key
}

// UserARN is the DataSafe principal ARN for a console user.
func UserARN(username string) string {
	return "arn:aws:iam::datasafe:user/" + username
}

// AccessKeyARN is the DataSafe principal ARN for an access key.
func AccessKeyARN(accessKey string) string {
	return "arn:aws:iam::datasafe:access-key/" + accessKey
}

// Evaluate returns true if the policy allows the action for principal.
// An empty policy allows any non-empty principal and denies anonymous.
// Deny overrides Allow. Principal and supported Condition keys are enforced.
func Evaluate(policyJSON, action, resource, principal string) bool {
	ctx := EvalContext{Action: action, Resource: resource}
	if principal == "" {
		ctx.Anonymous = true
	} else {
		ctx.Principals = []string{principal, UserARN(principal), AccessKeyARN(principal)}
	}
	return EvaluateContext(policyJSON, ctx)
}

// EvaluateContext applies Principal, Deny and supported conditions.
func EvaluateContext(policyJSON string, ctx EvalContext) bool {
	if policyJSON == "" {
		return !ctx.Anonymous && len(ctx.Principals) > 0
	}
	var doc Document
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return false
	}
	allowed := false
	for _, st := range doc.Statement {
		if !statementMatches(st, ctx) {
			continue
		}
		switch strings.EqualFold(st.Effect, "Deny") {
		case true:
			return false
		}
		if strings.EqualFold(st.Effect, "Allow") {
			allowed = true
		}
	}
	return allowed
}

func statementMatches(st Statement, ctx EvalContext) bool {
	if !principalMatches(st.Principal, ctx) {
		return false
	}
	if !actionMatches(st.Action, ctx.Action) {
		return false
	}
	if !resourceListMatches(st.Resource, ctx.Resource) {
		return false
	}
	return conditionMatches(st.Condition, ctx)
}

func actionMatches(actions StringList, action string) bool {
	for _, a := range actions {
		if a == action || a == "s3:*" || a == "*" {
			return true
		}
	}
	return false
}

func resourceListMatches(resources StringList, resource string) bool {
	for _, res := range resources {
		if resourceMatches(res, resource) {
			return true
		}
	}
	return false
}

func principalMatches(raw json.RawMessage, ctx EvalContext) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return principalTokenMatches(asString, ctx)
	}
	var asObj map[string]any
	if err := json.Unmarshal(raw, &asObj); err != nil {
		return false
	}
	aws, ok := asObj["AWS"]
	if !ok {
		return false
	}
	switch v := aws.(type) {
	case string:
		return principalTokenMatches(v, ctx)
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if ok && principalTokenMatches(s, ctx) {
				return true
			}
		}
	}
	return false
}

func principalTokenMatches(token string, ctx EvalContext) bool {
	token = strings.TrimSpace(token)
	if token == "*" {
		return true
	}
	if ctx.Anonymous {
		return false
	}
	for _, p := range ctx.Principals {
		if p == "" {
			continue
		}
		if token == p {
			return true
		}
		if strings.HasSuffix(token, "/"+p) && (strings.Contains(token, ":user/") || strings.Contains(token, ":access-key/")) {
			return true
		}
	}
	return false
}

func conditionMatches(cond map[string]map[string]any, ctx EvalContext) bool {
	if len(cond) == 0 {
		return true
	}
	for op, keys := range cond {
		for key, raw := range keys {
			values := conditionValues(raw)
			if !conditionKeyMatches(op, key, values, ctx) {
				return false
			}
		}
	}
	return true
}

func conditionValues(raw any) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func conditionKeyMatches(op, key string, values []string, ctx EvalContext) bool {
	switch strings.ToLower(op) {
	case "ipaddress":
		if !strings.EqualFold(key, "aws:SourceIp") {
			return false
		}
		return ipInAny(ctx.SourceIP, values)
	case "notipaddress":
		if !strings.EqualFold(key, "aws:SourceIp") {
			return false
		}
		return ctx.SourceIP != "" && !ipInAny(ctx.SourceIP, values)
	case "bool":
		if !strings.EqualFold(key, "aws:SecureTransport") {
			return false
		}
		want, ok := boolValues(values)
		if !ok {
			return false
		}
		return ctx.SecureTransport == want
	default:
		return false
	}
}

func boolValues(values []string) (bool, bool) {
	if len(values) != 1 {
		return false, false
	}
	switch strings.ToLower(values[0]) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func ipInAny(ip string, values []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			parsed = net.ParseIP(host)
		}
	}
	if parsed == nil {
		return false
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.Contains(v, "/") {
			_, network, err := net.ParseCIDR(v)
			if err == nil && network.Contains(parsed) {
				return true
			}
			continue
		}
		if other := net.ParseIP(v); other != nil && other.Equal(parsed) {
			return true
		}
	}
	return false
}

func resourceMatches(pattern, resource string) bool {
	if pattern == "*" || pattern == resource {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resource, prefix)
	}
	return false
}

// Denies reports whether a matching Deny statement applies.
// An empty policy does not deny. Invalid JSON is treated as a deny.
func Denies(policyJSON string, ctx EvalContext) bool {
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON == "" {
		return false
	}
	var doc Document
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return true
	}
	for _, st := range doc.Statement {
		if strings.EqualFold(st.Effect, "Deny") && statementMatches(st, ctx) {
			return true
		}
	}
	return false
}

// ErrUnsupportedCondition is returned when a policy uses a condition this
// server does not evaluate. Callers must reject the policy instead of
// ignoring the condition.
var ErrUnsupportedCondition = errors.New("unsupported condition")

// Validate rejects policies that use conditions other than aws:SourceIp and
// aws:SecureTransport. An empty policy is valid.
func Validate(policyJSON string) error {
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON == "" {
		return nil
	}
	var doc Document
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return fmt.Errorf("invalid policy json")
	}
	for _, st := range doc.Statement {
		effect := strings.ToLower(st.Effect)
		if effect != "allow" && effect != "deny" {
			return fmt.Errorf("statement effect must be Allow or Deny")
		}
		for op, keys := range st.Condition {
			switch strings.ToLower(op) {
			case "ipaddress", "notipaddress":
				for key := range keys {
					if !strings.EqualFold(key, "aws:SourceIp") {
						return fmt.Errorf("%w: %s %s", ErrUnsupportedCondition, op, key)
					}
				}
			case "bool":
				for key := range keys {
					if !strings.EqualFold(key, "aws:SecureTransport") {
						return fmt.Errorf("%w: %s %s", ErrUnsupportedCondition, op, key)
					}
				}
			default:
				return fmt.Errorf("%w: %s", ErrUnsupportedCondition, op)
			}
		}
	}
	return nil
}
