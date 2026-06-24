package policy

import (
	"encoding/json"
	"strings"
)

// Document is a minimal bucket policy subset.
type Document struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

type Statement struct {
	Effect    string   `json:"Effect"`
	Principal any      `json:"Principal"`
	Action    []string `json:"Action"`
	Resource  []string `json:"Resource"`
}

func BucketARN(bucket string) string {
	return "arn:aws:s3:::" + bucket
}

func ObjectARN(bucket, key string) string {
	return "arn:aws:s3:::" + bucket + "/" + key
}

// Evaluate returns true if the policy allows the action.
func Evaluate(policyJSON, action, resource, principal string) bool {
	if policyJSON == "" {
		return principal != ""
	}
	var doc Document
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return false
	}
	for _, st := range doc.Statement {
		if st.Effect != "Allow" {
			continue
		}
		for _, a := range st.Action {
			if a != action && a != "s3:*" {
				continue
			}
			for _, res := range st.Resource {
				if resourceMatches(res, resource) {
					return true
				}
			}
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
