package migrate

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/DirektorBani/datasafe/internal/policy"
)

// MapBucketPolicies turns MinIO canned policies and their user bindings into
// one DataSafe bucket policy per bucket. Unsupported conditions are named in
// the warnings and those statements are not copied.
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
		ported, dropped := PortPolicy(raw)
		for _, w := range dropped {
			warnings = append(warnings, "policy "+name+": "+w)
		}
		if ported == "" {
			continue
		}
		var doc policy.Document
		if err := json.Unmarshal([]byte(ported), &doc); err != nil {
			warnings = append(warnings, "policy "+name+" is not JSON and was not copied")
			continue
		}
		principal, err := json.Marshal(map[string][]string{"AWS": who})
		if err != nil {
			warnings = append(warnings, "policy "+name+" principals could not be encoded")
			continue
		}
		for _, st := range doc.Statement {
			buckets, all := bucketsFromResources(st.Resource)
			if all {
				warnings = append(warnings, "policy "+name+" applies to every bucket and was not copied; DataSafe stores a policy on a named bucket")
				continue
			}
			if len(buckets) == 0 {
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

func policyAllowsList(statements []policy.Statement, principal string) bool {
	for _, st := range statements {
		if !strings.EqualFold(st.Effect, "Allow") {
			continue
		}
		if !principalListed(st.Principal, principal) {
			continue
		}
		for _, action := range st.Action {
			switch strings.ToLower(action) {
			case "s3:listbucket", "s3:list*", "s3:*", "*":
				return true
			}
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
