package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const minioAdminPrefix = "/minio/admin/v3"

// IAMUser is one builtin MinIO user. SecretKey from the admin API is ignored.
type IAMUser struct {
	AccessKey  string
	PolicyName string
	Status     string
	MemberOf   []string
}

// IAMGroup is one MinIO group.
type IAMGroup struct {
	Name    string
	Status  string
	Members []string
	Policy  string
}

// IAMServiceAccount is a child key. Its MinIO secret is not available.
type IAMServiceAccount struct {
	Parent        string
	AccessKey     string
	Policy        string
	ImpliedPolicy bool
	Status        string
	// Expires is the source expiration, when MinIO reported one.
	Expires *time.Time
}

// IAMExternal is an LDAP or OIDC identity. It is not imported.
type IAMExternal struct {
	Kind   string
	Name   string
	Reason string
}

// IAMSnapshot is the MinIO admin view used by the import.
type IAMSnapshot struct {
	Users           []IAMUser
	Groups          []IAMGroup
	Policies        map[string]string
	ServiceAccounts []IAMServiceAccount
	External        []IAMExternal
	Warnings        []string
}

// ReadMinIOIAM lists users, groups, policies, and service accounts.
func ReadMinIOIAM(ctx context.Context, src Endpoint) (IAMSnapshot, error) {
	var snap IAMSnapshot
	snap.Policies = map[string]string{}
	raw, err := adminGet(ctx, src, minioAdminPrefix+"/list-users", nil)
	if err != nil {
		return snap, err
	}
	var users map[string]struct {
		PolicyName string   `json:"policyName"`
		Status     string   `json:"status"`
		MemberOf   []string `json:"memberOf"`
	}
	if err := json.Unmarshal(raw, &users); err != nil {
		return snap, err
	}
	for name, info := range users {
		snap.Users = append(snap.Users, IAMUser{
			AccessKey: name, PolicyName: info.PolicyName, Status: info.Status, MemberOf: info.MemberOf,
		})
	}
	if groups, err := adminGet(ctx, src, minioAdminPrefix+"/groups", nil); err == nil {
		snap.Groups = decodeGroups(groups)
	} else if !adminMissing(err) {
		return snap, err
	} else {
		snap.Warnings = append(snap.Warnings, "MinIO groups endpoint was not available; no groups were imported")
	}
	for i, g := range snap.Groups {
		// Real MinIO serves GET /group?group=name. /group-desc answers 426 and
		// must not abort the import.
		desc, err := adminGet(ctx, src, minioAdminPrefix+"/group", url.Values{"group": {g.Name}})
		if err != nil {
			if adminMissing(err) {
				snap.Warnings = append(snap.Warnings, "group "+g.Name+" has no description and its members were not copied")
				continue
			}
			return snap, err
		}
		var body struct {
			Members     []string `json:"members"`
			Policy      string   `json:"policy"`
			GroupPolicy string   `json:"groupPolicy"`
			Status      string   `json:"status"`
			GroupStatus string   `json:"groupStatus"`
		}
		if json.Unmarshal(desc, &body) == nil {
			if len(body.Members) > 0 {
				snap.Groups[i].Members = body.Members
			}
			switch {
			case body.Policy != "":
				snap.Groups[i].Policy = body.Policy
			case body.GroupPolicy != "":
				snap.Groups[i].Policy = body.GroupPolicy
			}
			switch {
			case body.Status != "":
				snap.Groups[i].Status = body.Status
			case body.GroupStatus != "":
				snap.Groups[i].Status = body.GroupStatus
			}
		}
	}
	if pols, err := adminGet(ctx, src, minioAdminPrefix+"/list-canned-policies", nil); err == nil {
		snap.Policies = decodePolicies(pols)
	} else if !adminMissing(err) {
		return snap, err
	} else {
		snap.Warnings = append(snap.Warnings, "MinIO policy endpoint was not available; no policies were imported")
	}
	for _, u := range snap.Users {
		accounts, err := adminGet(ctx, src, minioAdminPrefix+"/list-service-accounts", url.Values{"user": {u.AccessKey}})
		if err != nil {
			if adminMissing(err) {
				continue
			}
			return snap, err
		}
		var list struct {
			Accounts []struct {
				AccessKey  string     `json:"accessKey"`
				Expiration *time.Time `json:"expiration"`
			} `json:"accounts"`
		}
		if json.Unmarshal(accounts, &list) != nil {
			continue
		}
		for _, acc := range list.Accounts {
			sa := IAMServiceAccount{Parent: u.AccessKey, AccessKey: acc.AccessKey, ImpliedPolicy: true, Status: "enabled", Expires: acc.Expiration}
			info, ierr := adminGet(ctx, src, minioAdminPrefix+"/info-service-account", url.Values{"accessKey": {acc.AccessKey}})
			if ierr == nil {
				var body struct {
					Policy        json.RawMessage `json:"policy"`
					ImpliedPolicy *bool           `json:"impliedPolicy"`
					AccountStatus string          `json:"accountStatus"`
					Expiration    *time.Time      `json:"expiration"`
				}
				if json.Unmarshal(info, &body) == nil {
					sa.Policy = rawJSONString(body.Policy)
					if body.ImpliedPolicy != nil {
						sa.ImpliedPolicy = *body.ImpliedPolicy
					} else if strings.TrimSpace(sa.Policy) != "" {
						sa.ImpliedPolicy = false
					}
					if body.AccountStatus != "" {
						sa.Status = body.AccountStatus
					}
					if body.Expiration != nil {
						sa.Expires = body.Expiration
					}
				}
			}
			snap.ServiceAccounts = append(snap.ServiceAccounts, sa)
		}
	}
	snap.External = append(snap.External, readExternal(ctx, src, minioAdminPrefix+"/idp/ldap/list-users", "ldap", "Configure LDAP on DataSafe. MinIO LDAP users are not imported.")...)
	snap.External = append(snap.External, readExternal(ctx, src, minioAdminPrefix+"/idp/openid/list-users", "oidc", "Configure OIDC on DataSafe. MinIO OIDC users are not imported.")...)
	return snap, nil
}

func readExternal(ctx context.Context, src Endpoint, path, kind, reason string) []IAMExternal {
	raw, err := adminGet(ctx, src, path, nil)
	if err != nil {
		return nil
	}
	var names []string
	if json.Unmarshal(raw, &names) != nil {
		var box struct {
			Users []string `json:"users"`
		}
		var asMap map[string]json.RawMessage
		switch {
		case json.Unmarshal(raw, &box) == nil && len(box.Users) > 0:
			names = box.Users
		case json.Unmarshal(raw, &asMap) == nil:
			for name := range asMap {
				names = append(names, name)
			}
		default:
			return nil
		}
	}
	var out []IAMExternal
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, IAMExternal{Kind: kind, Name: name, Reason: reason})
	}
	return out
}

func decodeGroups(raw []byte) []IAMGroup {
	var names []string
	if json.Unmarshal(raw, &names) == nil {
		var out []IAMGroup
		for _, name := range names {
			if name != "" {
				out = append(out, IAMGroup{Name: name, Status: "enabled"})
			}
		}
		return out
	}
	var box struct {
		Groups []string `json:"groups"`
	}
	if json.Unmarshal(raw, &box) == nil {
		var out []IAMGroup
		for _, name := range box.Groups {
			if name != "" {
				out = append(out, IAMGroup{Name: name, Status: "enabled"})
			}
		}
		return out
	}
	return nil
}

func decodePolicies(raw []byte) map[string]string {
	out := map[string]string{}
	var asRaw map[string]json.RawMessage
	if json.Unmarshal(raw, &asRaw) != nil {
		return out
	}
	for name, body := range asRaw {
		body = bytesTrim(body)
		if len(body) == 0 || string(body) == "null" {
			continue
		}
		if body[0] == '"' {
			var s string
			if json.Unmarshal(body, &s) == nil {
				out[name] = s
			}
			continue
		}
		out[name] = string(body)
	}
	return out
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

type adminError struct {
	Status int
	Body   string
}

func (e *adminError) Error() string {
	if e.Body == "" {
		return "minio admin http " + itoa(e.Status)
	}
	return "minio admin http " + itoa(e.Status) + ": " + e.Body
}

func adminMissing(err error) bool {
	var ae *adminError
	if !errors.As(err, &ae) {
		return false
	}
	// 426 is what a current MinIO returns for the retired /group-desc path.
	return ae.Status == http.StatusNotFound || ae.Status == http.StatusMethodNotAllowed || ae.Status == http.StatusUpgradeRequired
}

func rawJSONString(raw json.RawMessage) string {
	raw = bytesTrim(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return string(raw)
}

func itoa(n int) string { return strconv.Itoa(n) }

func adminGet(ctx context.Context, src Endpoint, path string, q url.Values) ([]byte, error) {
	resp, err := src.do(ctx, http.MethodGet, path, q, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &adminError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	plain, err := DecryptAdmin(src.SecretKey, body)
	if err != nil {
		return nil, err
	}
	return plain, nil
}
