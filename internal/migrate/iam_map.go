package migrate

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/policy"
)

// MapBucketPolicies turns MinIO canned policies and their user bindings into
// one DataSafe bucket policy per bucket. An Allow that cannot be represented
// exactly is dropped. A Deny with an unsupported condition is kept with that
// condition removed, so it denies unconditionally.
func MapBucketPolicies(docs map[string]string, principals map[string][]string) (map[string][]policy.Statement, []string) {
	var warnings []string
	out := map[string][]policy.Statement{}
	names := make([]string, 0, len(docs))
	for name := range docs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := strings.TrimSpace(docs[name])
		if raw == "" {
			warnings = append(warnings, "policy "+name+" has no document and was not copied")
			continue
		}
		who := uniqueStrings(principals[name])
		if len(who) == 0 {
			warnings = append(warnings, "policy "+name+" has no user binding and was not copied")
			continue
		}
		stmts, dropped, refuse := portIAMDocument(raw)
		for _, w := range dropped {
			warnings = append(warnings, "policy "+name+": "+w)
		}
		if refuse {
			warnings = append(warnings, "policy "+name+" has a deny that cannot be represented; its allows were not copied")
			continue
		}
		principal, err := json.Marshal(map[string][]string{"AWS": who})
		if err != nil {
			warnings = append(warnings, "policy "+name+" principals could not be encoded")
			continue
		}
		for _, st := range stmts {
			buckets, all := bucketsFromResources(st.Resource)
			if all {
				if strings.EqualFold(st.Effect, "Deny") {
					warnings = append(warnings, "policy "+name+" deny applies to every bucket and cannot be stored; principals of this policy are refused")
					continue
				}
				warnings = append(warnings, "policy "+name+" applies to every bucket and was not copied; DataSafe stores a policy on a named bucket")
				continue
			}
			if len(buckets) == 0 {
				if strings.EqualFold(st.Effect, "Deny") {
					warnings = append(warnings, "policy "+name+" deny has no bucket resource and cannot be stored; principals of this policy are refused")
					continue
				}
				warnings = append(warnings, "policy "+name+" has no bucket resource and was not copied")
				continue
			}
			st.Principal = principal
			for _, bucket := range buckets {
				out[bucket] = append(out[bucket], st)
			}
		}
	}
	return out, warnings
}

// portIAMDocument keeps statements DataSafe can enforce without widening them.
// refuse is set when a Deny cannot be attached to a named bucket.
func portIAMDocument(raw string) (stmts []policy.Statement, warnings []string, refuse bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, false
	}
	var doc policy.Document
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return nil, []string{"policy is not JSON and was not copied"}, false
	}
	for _, st := range doc.Statement {
		effect := strings.ToLower(strings.TrimSpace(st.Effect))
		sid := st.Sid
		if sid == "" {
			sid = strings.Join(st.Action, ",")
		}
		if effect != "allow" && effect != "deny" {
			warnings = append(warnings, "statement "+sid+" was not copied: effect must be Allow or Deny")
			continue
		}
		bad := unsupportedCondition(st)
		if len(bad) > 0 {
			if effect == "deny" {
				st.Condition = nil
				warnings = append(warnings, "deny statement "+sid+" is enforced unconditionally; unsupported condition dropped: "+strings.Join(bad, ", "))
			} else {
				warnings = append(warnings, "allow statement "+sid+" dropped: "+strings.Join(bad, ", "))
				continue
			}
		}
		buckets, all := bucketsFromResources(st.Resource)
		if effect == "deny" && (all || len(buckets) == 0) {
			warnings = append(warnings, "deny statement "+sid+" cannot be represented on a named bucket")
			refuse = true
			continue
		}
		if effect == "allow" && (all || len(buckets) == 0) {
			warnings = append(warnings, "allow statement "+sid+" applies to every bucket and was not copied")
			continue
		}
		stmts = append(stmts, st)
	}
	return stmts, warnings, refuse
}

func unsupportedCondition(st policy.Statement) []string {
	var bad []string
	for op, keys := range st.Condition {
		switch strings.ToLower(op) {
		case "ipaddress", "notipaddress":
			for key := range keys {
				if !strings.EqualFold(key, "aws:SourceIp") {
					bad = append(bad, op+" "+key)
				}
			}
		case "bool":
			for key := range keys {
				if !strings.EqualFold(key, "aws:SecureTransport") {
					bad = append(bad, op+" "+key)
				}
			}
		default:
			named := false
			for key := range keys {
				bad = append(bad, op+" "+key)
				named = true
			}
			if !named {
				bad = append(bad, op)
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// userStatements ports each user's policies and enabled group policies.
// A user in refused has a Deny that cannot be stored; that user gets no key.
// MinIO returns no policy at all for a member of a disabled group, which denies
// every request, including that user's own allows. Those allows are replaced
// with an unconditional deny on each named bucket.
func userStatements(snap IAMSnapshot) (userStmts map[string][]policy.Statement, refused map[string]string, warnings []string) {
	userStmts = map[string][]policy.Statement{}
	refused = map[string]string{}
	for _, u := range snap.Users {
		names := splitPolicies(u.PolicyName)
		var scan []string
		scan = append(scan, names...)
		var disabled []string
		for _, g := range groupsOf(u, snap.Groups) {
			if strings.TrimSpace(g.Policy) != "" {
				scan = append(scan, splitPolicies(g.Policy)...)
			}
			if !accountEnabled(g.Status) {
				disabled = append(disabled, g.Name)
				continue
			}
			if strings.TrimSpace(g.Policy) != "" {
				names = append(names, splitPolicies(g.Policy)...)
			}
		}
		var stmts []policy.Statement
		bad := false
		for _, name := range uniqueStrings(names) {
			raw := snap.Policies[name]
			if strings.TrimSpace(raw) == "" {
				warnings = append(warnings, "policy "+name+" has no document and was not copied")
				continue
			}
			st, w, refuse := portIAMDocument(raw)
			for _, line := range w {
				warnings = append(warnings, "policy "+name+": "+line)
			}
			if refuse {
				bad = true
				warnings = append(warnings, "user "+u.AccessKey+" is refused: policy "+name+" has a deny that cannot be represented")
			}
			if !bad {
				stmts = append(stmts, st...)
			}
		}
		if bad {
			refused[u.AccessKey] = "deny cannot be represented"
			continue
		}
		if len(disabled) > 0 {
			sort.Strings(disabled)
			buckets := namedBucketsInPolicies(snap.Policies, scan)
			if len(buckets) == 0 {
				reason := "member of disabled group " + strings.Join(disabled, ", ") + "; MinIO denies every request and that deny cannot be attached to a named bucket"
				refused[u.AccessKey] = reason
				warnings = append(warnings, "user "+u.AccessKey+" is refused: "+reason)
				continue
			}
			stmts = denyBuckets(buckets)
			warnings = append(warnings, "user "+u.AccessKey+" is a member of disabled group "+strings.Join(disabled, ", ")+"; MinIO denies every request, so allows were dropped and access is denied")
		}
		userStmts[u.AccessKey] = stmts
	}
	return userStmts, refused, warnings
}

// denyBuckets is an unconditional deny of every S3 action on the bucket and
// its objects. It is stricter than dropping the user's allows and leaving the
// key unbound, which would still permit ListAllMyBuckets.
func denyBuckets(buckets []string) []policy.Statement {
	out := make([]policy.Statement, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, policy.Statement{
			Sid:      "disabled-group",
			Effect:   "Deny",
			Action:   []string{"s3:*"},
			Resource: []string{"arn:aws:s3:::" + bucket, "arn:aws:s3:::" + bucket + "/*"},
		})
	}
	return out
}

func namedBucketsInPolicies(policies map[string]string, names []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, name := range uniqueStrings(names) {
		raw := strings.TrimSpace(policies[name])
		if raw == "" {
			continue
		}
		var doc policy.Document
		if json.Unmarshal([]byte(raw), &doc) != nil {
			continue
		}
		for _, st := range doc.Statement {
			for _, res := range st.Resource {
				bucket, ok := bucketFromResource(res)
				if !ok {
					continue
				}
				if _, dup := seen[bucket]; dup {
					continue
				}
				seen[bucket] = struct{}{}
				out = append(out, bucket)
			}
		}
	}
	sort.Strings(out)
	return out
}

func bucketFromResource(res string) (string, bool) {
	res = strings.TrimSpace(res)
	switch res {
	case "", "*", "arn:aws:s3:::*", "arn:aws:s3:::*/*":
		return "", false
	}
	const prefix = "arn:aws:s3:::"
	if !strings.HasPrefix(res, prefix) {
		return "", false
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(res, prefix), "/")
	if name == "" || name == "*" {
		return "", false
	}
	return name, true
}

// EffectiveIAM binds each imported access key to statements that are equal to
// or stricter than MinIO. A service account with an inline policy does not
// inherit parent allows the inline policy does not itself grant.
func EffectiveIAM(snap IAMSnapshot, userStmts map[string][]policy.Statement, refusedUser map[string]string, userKeys map[string]string, saKeys map[string]string, now time.Time) (docs map[string]string, principals map[string][]string, warnings []string, groups []IAMGroupNote, accounts []IAMServiceAccountNote, refused []string) {
	docs = map[string]string{}
	principals = map[string][]string{}
	if userStmts == nil {
		userStmts = map[string][]policy.Statement{}
	}
	if refusedUser == nil {
		refusedUser = map[string]string{}
	}
	for user := range refusedUser {
		refused = append(refused, user)
	}
	for _, g := range snap.Groups {
		note := IAMGroupNote{
			Name: g.Name, Status: g.Status, Policy: g.Policy, Members: append([]string(nil), g.Members...),
		}
		if note.Status == "" {
			note.Status = "enabled"
		}
		if accountEnabled(g.Status) {
			note.PolicyApplied = strings.TrimSpace(g.Policy) != ""
			note.Detail = "group policy is expanded to member access keys; DataSafe bucket policies have no group principal"
			if !note.PolicyApplied {
				note.Detail = "group has no policy"
			}
		} else {
			note.PolicyApplied = false
			note.Detail = "group is disabled; its policy was not applied. MinIO denies every request for a member, so those members' allows were dropped"
		}
		groups = append(groups, note)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	for user, stmts := range userStmts {
		ak := userKeys[user]
		if ak == "" || len(stmts) == 0 {
			continue
		}
		id := "user:" + user
		raw, err := json.Marshal(policy.Document{Version: "2012-10-17", Statement: stmts})
		if err != nil {
			warnings = append(warnings, "user "+user+" policy could not be encoded")
			continue
		}
		docs[id] = string(raw)
		principals[id] = []string{policy.AccessKeyARN(ak)}
	}
	for _, sa := range snap.ServiceAccounts {
		note := IAMServiceAccountNote{
			AccessKey: sa.AccessKey, Parent: sa.Parent, Status: sa.Status, ImpliedPolicy: sa.ImpliedPolicy,
		}
		if note.Status == "" {
			note.Status = "enabled"
		}
		if sa.Expires != nil {
			note.Expiration = sa.Expires.UTC().Format(time.RFC3339)
		}
		switch {
		case !accountEnabled(sa.Status):
			note.Inline = "disabled"
			note.Detail = "service account is disabled; no access key was created"
			warnings = append(warnings, "service account "+sa.AccessKey+" is disabled; no access key was created")
		case sa.Expires != nil && !sa.Expires.After(now):
			note.Inline = "expired"
			note.Detail = "service account is expired; no access key was created"
			warnings = append(warnings, "service account "+sa.AccessKey+" is expired; no access key was created")
		case refusedUser[sa.Parent] != "":
			note.Inline = "refused"
			note.Detail = "parent user was refused; service account was not imported"
		case userKeys[sa.Parent] == "" && saKeys[sa.AccessKey] == "":
			note.Inline = "skipped"
			note.Detail = "parent user has no access key"
		default:
			parent := userStmts[sa.Parent]
			ak := saKeys[sa.AccessKey]
			var stmts []policy.Statement
			if !sa.ImpliedPolicy && strings.TrimSpace(sa.Policy) != "" {
				if _, _, refuse := portIAMDocument(sa.Policy); refuse {
					note.Inline = "refused"
					note.Detail = "inline deny cannot be represented; no access key was created"
					warnings = append(warnings, "service account "+sa.AccessKey+" is refused: inline deny cannot be represented")
					refused = append(refused, sa.AccessKey)
					accounts = append(accounts, note)
					continue
				}
			}
			if ak == "" {
				note.Inline = "skipped"
				note.Detail = "no access key was created"
				accounts = append(accounts, note)
				continue
			}
			if sa.ImpliedPolicy || strings.TrimSpace(sa.Policy) == "" {
				note.ImpliedPolicy = true
				note.Inline = "implied"
				note.Detail = "service account uses the parent policy; the MinIO secret was not available"
				stmts = parent
				if ak != "" {
					warnings = append(warnings, "service account "+sa.AccessKey+" secret was not available; a new key was issued for "+sa.Parent)
				}
			} else {
				inline, w, refuse := portIAMDocument(sa.Policy)
				for _, line := range w {
					warnings = append(warnings, "service account "+sa.AccessKey+" inline policy: "+line)
				}
				if refuse {
					note.Inline = "refused"
					note.Detail = "inline deny cannot be represented; no access key was created"
					warnings = append(warnings, "service account "+sa.AccessKey+" is refused: inline deny cannot be represented")
					refused = append(refused, sa.AccessKey)
					accounts = append(accounts, note)
					continue
				}
				var narrowed []string
				stmts, narrowed = intersectAllows(parent, inline)
				note.Inline = "applied"
				note.Detail = "inline policy replaces parent allows for this key; the MinIO secret was not available"
				if len(narrowed) > 0 {
					note.Inline = "narrowed"
					note.Detail = "inline policy narrowed: " + strings.Join(narrowed, "; ")
					warnings = append(warnings, "service account "+sa.AccessKey+" inline policy narrowed: "+strings.Join(narrowed, "; "))
				}
				warnings = append(warnings, "service account "+sa.AccessKey+" secret was not available; a new key was issued for "+sa.Parent+" and the inline policy is the allow set")
			}
			if ak != "" && len(stmts) > 0 {
				id := "sa:" + sa.AccessKey
				raw, err := json.Marshal(policy.Document{Version: "2012-10-17", Statement: stmts})
				if err != nil {
					warnings = append(warnings, "service account "+sa.AccessKey+" policy could not be encoded")
				} else {
					docs[id] = string(raw)
					principals[id] = []string{policy.AccessKeyARN(ak)}
				}
			}
		}
		accounts = append(accounts, note)
	}
	sort.Strings(refused)
	return docs, principals, warnings, groups, accounts, refused
}

func intersectAllows(parent, inline []policy.Statement) ([]policy.Statement, []string) {
	var out []policy.Statement
	var warnings []string
	for _, st := range inline {
		if strings.EqualFold(st.Effect, "Deny") {
			out = append(out, st)
			continue
		}
		if !strings.EqualFold(st.Effect, "Allow") {
			continue
		}
		cover, ok := coveringAllow(parent, st)
		if !ok {
			sid := st.Sid
			if sid == "" {
				sid = strings.Join(st.Action, ",")
			}
			warnings = append(warnings, "allow "+sid+" is outside the parent policy and was dropped")
			continue
		}
		st.Condition = mergeConditions(st.Condition, cover.Condition)
		out = append(out, st)
	}
	for _, st := range parent {
		if strings.EqualFold(st.Effect, "Deny") {
			out = append(out, st)
		}
	}
	return out, warnings
}

func coveringAllow(parent []policy.Statement, child policy.Statement) (policy.Statement, bool) {
	for _, st := range parent {
		if !strings.EqualFold(st.Effect, "Allow") {
			continue
		}
		if !actionsCovered(st.Action, child.Action) || !resourcesCovered(st.Resource, child.Resource) {
			continue
		}
		return st, true
	}
	return policy.Statement{}, false
}

func actionsCovered(parent, child []string) bool {
	if len(child) == 0 {
		return false
	}
	for _, c := range child {
		ok := false
		for _, p := range parent {
			if p == "*" || p == "s3:*" || p == c {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func resourcesCovered(parent, child []string) bool {
	if len(child) == 0 {
		return false
	}
	for _, c := range child {
		ok := false
		for _, p := range parent {
			if resourceCovers(p, c) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func resourceCovers(parent, child string) bool {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	switch parent {
	case "*", "arn:aws:s3:::*", "arn:aws:s3:::*/*":
		return true
	}
	if parent == child {
		return true
	}
	if strings.HasSuffix(parent, "/*") {
		prefix := strings.TrimSuffix(parent, "*")
		return strings.HasPrefix(child, prefix) || child == strings.TrimSuffix(prefix, "/")
	}
	return false
}

func mergeConditions(a, b map[string]map[string]any) map[string]map[string]any {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := map[string]map[string]any{}
	for _, src := range []map[string]map[string]any{a, b} {
		for op, keys := range src {
			if out[op] == nil {
				out[op] = map[string]any{}
			}
			for k, v := range keys {
				out[op][k] = v
			}
		}
	}
	return out
}

func accountEnabled(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "enabled", "on", "active":
		return true
	default:
		return false
	}
}

// MergeBucketPolicy appends statements that are not already present.
// Statements already stored are left in place. Actions, resources, principals,
// and conditions are compared in canonical form, and the stored document is
// rewritten in that form so a second merge of the same statements is byte-stable.
func MergeBucketPolicy(existing string, add []policy.Statement) (string, error) {
	var doc policy.Document
	existing = strings.TrimSpace(existing)
	if existing != "" {
		if err := json.Unmarshal([]byte(existing), &doc); err != nil {
			return "", err
		}
	}
	seen := map[string]policy.Statement{}
	var order []string
	addOne := func(st policy.Statement) error {
		canon, key, err := canonicalStatement(st)
		if err != nil {
			return err
		}
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = canon
		order = append(order, key)
		return nil
	}
	for _, st := range doc.Statement {
		if err := addOne(st); err != nil {
			return "", err
		}
	}
	for _, st := range add {
		if err := addOne(st); err != nil {
			return "", err
		}
	}
	sort.Strings(order)
	doc.Statement = doc.Statement[:0]
	for _, key := range order {
		doc.Statement = append(doc.Statement, seen[key])
	}
	if doc.Version == "" {
		doc.Version = "2012-10-17"
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func canonicalStatement(st policy.Statement) (policy.Statement, string, error) {
	switch strings.ToLower(strings.TrimSpace(st.Effect)) {
	case "allow":
		st.Effect = "Allow"
	case "deny":
		st.Effect = "Deny"
	}
	st.Sid = strings.TrimSpace(st.Sid)
	st.Action = sortedCopy(st.Action)
	st.Resource = sortedCopy(st.Resource)
	st.Principal = canonicalPrincipal(st.Principal)
	st.Condition = canonicalCondition(st.Condition)
	raw, err := json.Marshal(st)
	if err != nil {
		return st, "", err
	}
	return st, string(raw), nil
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func canonicalPrincipal(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		b, _ := json.Marshal(map[string][]string{"AWS": {asString}})
		return b
	}
	var box struct {
		AWS json.RawMessage `json:"AWS"`
	}
	if json.Unmarshal(raw, &box) != nil || len(bytes.TrimSpace(box.AWS)) == 0 {
		return raw
	}
	var many []string
	var one string
	if json.Unmarshal(box.AWS, &one) == nil {
		many = []string{one}
	} else if json.Unmarshal(box.AWS, &many) != nil {
		return raw
	}
	many = uniqueStrings(many)
	b, err := json.Marshal(map[string][]string{"AWS": many})
	if err != nil {
		return raw
	}
	return b
}

func canonicalCondition(in map[string]map[string]any) map[string]map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := map[string]map[string]any{}
	for op, keys := range in {
		op = strings.ToLower(strings.TrimSpace(op))
		if op == "" {
			continue
		}
		dst := map[string]any{}
		for key, value := range keys {
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "" {
				continue
			}
			vals := conditionStrings(value)
			sort.Strings(vals)
			dst[key] = vals
		}
		if len(dst) > 0 {
			out[op] = dst
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func conditionStrings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case bool:
		if t {
			return []string{"true"}
		}
		return []string{"false"}
	case []string:
		return append([]string(nil), t...)
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, conditionStrings(item)...)
		}
		return out
	default:
		if t == nil {
			return nil
		}
		b, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		return []string{string(b)}
	}
}

func bucketsFromResources(resources []string) (buckets []string, all bool) {
	seen := map[string]struct{}{}
	for _, res := range resources {
		res = strings.TrimSpace(res)
		switch res {
		case "*", "arn:aws:s3:::*", "arn:aws:s3:::*/*":
			return nil, true
		}
		const prefix = "arn:aws:s3:::"
		if !strings.HasPrefix(res, prefix) {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(res, prefix), "/")
		if name == "" || name == "*" {
			return nil, true
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		buckets = append(buckets, name)
	}
	sort.Strings(buckets)
	return buckets, false
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// listProbePlan reports whether this principal's statements mention ListBucket
// on the bucket, and whether a probe from this host is expected to be allowed.
// An Allow whose condition does not match (for example aws:SourceIp) is a deny.
func listProbePlan(statements []policy.Statement, principal, bucket string, secure bool) (probe bool, allow bool) {
	var mine []policy.Statement
	for _, st := range statements {
		if !principalListed(st.Principal, principal) || !statementListsBucket(st) {
			continue
		}
		mine = append(mine, st)
	}
	if len(mine) == 0 {
		return false, false
	}
	raw, err := json.Marshal(policy.Document{Version: "2012-10-17", Statement: mine})
	if err != nil {
		return true, false
	}
	allow = policy.EvaluateContext(string(raw), policy.EvalContext{
		Action:          "s3:ListBucket",
		Resource:        policy.BucketARN(bucket),
		Principals:      []string{principal},
		SecureTransport: secure,
	})
	return true, allow
}

func statementListsBucket(st policy.Statement) bool {
	for _, action := range st.Action {
		switch strings.ToLower(action) {
		case "s3:listbucket", "s3:list*", "s3:*", "*":
			return true
		}
	}
	return false
}

func principalListed(raw json.RawMessage, want string) bool {
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return asString == "*" || asString == want
	}
	var box struct {
		AWS json.RawMessage `json:"AWS"`
	}
	if json.Unmarshal(raw, &box) != nil {
		return false
	}
	var one string
	if json.Unmarshal(box.AWS, &one) == nil {
		return one == "*" || one == want
	}
	var many []string
	if json.Unmarshal(box.AWS, &many) != nil {
		return false
	}
	for _, p := range many {
		if p == "*" || p == want {
			return true
		}
	}
	return false
}
