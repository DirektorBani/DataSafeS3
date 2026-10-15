package migrate

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/policy"
)

// IAMOptions configures storage-cli migrate from-minio iam.
type IAMOptions struct {
	Source         Endpoint
	Targets        []Endpoint
	AdminUser      string
	AdminPassword  string
	Tenant         string
	KeysFile       string
	KeepAccessKeys string
	ReportDir      string
	Stderr         io.Writer
}

// IAMProbe is one signed request made with an imported key.
type IAMProbe struct {
	Username string `json:"username"`
	Bucket   string `json:"bucket"`
	Action   string `json:"action"`
	Status   int    `json:"status"`
	Verdict  string `json:"verdict"`
}

// IAMSkip is something the import did not copy, with the reason.
type IAMSkip struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// IAMGroupNote is one group in the import report.
type IAMGroupNote struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Policy        string   `json:"policy,omitempty"`
	Members       []string `json:"members,omitempty"`
	PolicyApplied bool     `json:"policy_applied"`
	Detail        string   `json:"detail,omitempty"`
}

// IAMServiceAccountNote records an imported or skipped service account.
type IAMServiceAccountNote struct {
	AccessKey     string `json:"access_key"`
	Parent        string `json:"parent"`
	Status        string `json:"status"`
	Expiration    string `json:"expiration,omitempty"`
	ImpliedPolicy bool   `json:"implied_policy"`
	Inline        string `json:"inline"`
	Detail        string `json:"detail,omitempty"`
}

// IAMReport is evidence.json for an IAM import.
type IAMReport struct {
	UsersTransferred    int                     `json:"users_transferred"`
	GroupsTransferred   int                     `json:"groups_transferred"`
	PoliciesTransferred int                     `json:"policies_transferred"`
	Warnings            []string                `json:"warnings,omitempty"`
	Errors              []string                `json:"errors,omitempty"`
	NotTransferred      []IAMSkip               `json:"not_transferred,omitempty"`
	Groups              []IAMGroupNote          `json:"groups,omitempty"`
	ServiceAccounts     []IAMServiceAccountNote `json:"service_accounts,omitempty"`
	Probes              []IAMProbe              `json:"probes,omitempty"`
	Verdicts            map[string]string       `json:"verdicts"`
}

type issuedKey struct {
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Source    string `json:"source,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Status    string `json:"status,omitempty"`
}

type keptKey struct {
	Username       string `json:"username"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	ServiceAccount string `json:"service_account,omitempty"`
}

// ParseIAM reads `migrate from-minio iam` arguments.
func ParseIAM(args []string) (IAMOptions, error) {
	opt := IAMOptions{
		AdminUser: "admin",
		KeysFile:  "./minio-iam-keys.json",
		ReportDir: "./minio-iam-report",
		Source:    Endpoint{Region: "us-east-1"},
	}
	var targets []string
	region := "us-east-1"
	for i := 0; i < len(args); i++ {
		a := args[i]
		need := func() (string, error) {
			i++
			if i >= len(args) {
				return "", fmt.Errorf("%s needs a value", a)
			}
			return args[i], nil
		}
		switch a {
		case "--source":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.URL = v
		case "--source-key":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.AccessKey = v
		case "--source-secret":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.SecretKey = v
		case "--source-secret-file":
			v, err := need()
			if err != nil {
				return opt, err
			}
			secret, err := readSecretFile(v)
			if err != nil {
				return opt, err
			}
			opt.Source.SecretKey = secret
		case "--source-region", "--region":
			v, err := need()
			if err != nil {
				return opt, err
			}
			region = v
		case "--target":
			v, err := need()
			if err != nil {
				return opt, err
			}
			targets = append(targets, v)
		case "--target-admin-user":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.AdminUser = v
		case "--target-admin-password":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.AdminPassword = v
		case "--target-admin-password-file":
			v, err := need()
			if err != nil {
				return opt, err
			}
			secret, err := readSecretFile(v)
			if err != nil {
				return opt, err
			}
			opt.AdminPassword = secret
		case "--tenant":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Tenant = v
		case "--keys-file":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.KeysFile = v
		case "--keep-access-keys":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.KeepAccessKeys = v
		case "--report":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.ReportDir = v
		default:
			return opt, fmt.Errorf("unknown flag %q", a)
		}
	}
	if opt.Source.AccessKey == "" {
		opt.Source.AccessKey = os.Getenv("SOURCE_ACCESS_KEY")
	}
	if opt.Source.SecretKey == "" {
		opt.Source.SecretKey = os.Getenv("SOURCE_SECRET_KEY")
	}
	if opt.AdminPassword == "" {
		opt.AdminPassword = os.Getenv("TARGET_ADMIN_PASSWORD")
	}
	if opt.AdminUser == "admin" && os.Getenv("TARGET_ADMIN_USER") != "" {
		opt.AdminUser = os.Getenv("TARGET_ADMIN_USER")
	}
	opt.Source.Region = region
	if opt.Source.URL == "" {
		return opt, fmt.Errorf("--source is required")
	}
	if len(targets) == 0 {
		return opt, fmt.Errorf("--target is required")
	}
	if opt.Source.AccessKey == "" || opt.Source.SecretKey == "" {
		return opt, fmt.Errorf("--source-key and --source-secret are required")
	}
	if opt.AdminUser == "" || opt.AdminPassword == "" {
		return opt, fmt.Errorf("--target-admin-user and --target-admin-password are required")
	}
	for _, u := range targets {
		opt.Targets = append(opt.Targets, Endpoint{URL: u, Region: region})
	}
	return opt, nil
}

// RunIAM copies MinIO IAM into DataSafe. MinIO user secrets are not copied.
func RunIAM(ctx context.Context, opt IAMOptions) (IAMReport, error) {
	if opt.Stderr == nil {
		opt.Stderr = io.Discard
	}
	rep := IAMReport{Verdicts: map[string]string{}}
	snap, err := ReadMinIOIAM(ctx, opt.Source)
	if err != nil {
		return rep, err
	}
	rep.Warnings = append(rep.Warnings, snap.Warnings...)
	for _, ext := range snap.External {
		rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: ext.Kind, Name: ext.Name, Reason: ext.Reason})
		rep.Warnings = append(rep.Warnings, ext.Kind+" "+ext.Name+": "+ext.Reason)
	}
	if len(snap.Groups) > 0 {
		rep.Warnings = append(rep.Warnings, "group policies are expanded to member users; DataSafe bucket policies have no group principal")
	}
	kept, err := readKeptKeys(opt.KeepAccessKeys)
	if err != nil {
		return rep, err
	}
	ds := &dsClient{targets: opt.Targets, user: opt.AdminUser, pass: opt.AdminPassword}
	if err := ds.login(ctx); err != nil {
		return rep, err
	}
	users := map[string]string{}
	passwords := map[string]string{}
	sort.Slice(snap.Users, func(i, j int) bool { return snap.Users[i].AccessKey < snap.Users[j].AccessKey })
	for _, u := range snap.Users {
		status := "active"
		if strings.EqualFold(u.Status, "disabled") {
			status = "disabled"
		}
		pass := randomSecret()
		id, existed, cerr := ds.createUser(ctx, u.AccessKey, pass, status, opt.Tenant)
		if cerr != nil {
			rep.Errors = append(rep.Errors, "user "+u.AccessKey+": "+cerr.Error())
			rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "user", Name: u.AccessKey, Reason: cerr.Error()})
			continue
		}
		if existed {
			rep.Warnings = append(rep.Warnings, "user "+u.AccessKey+" already exists; a new password was not set")
			pass = ""
		}
		users[u.AccessKey] = id
		passwords[u.AccessKey] = pass
		rep.UsersTransferred++
		if strings.Contains(strings.ToLower(u.PolicyName), "consoleadmin") {
			rep.Warnings = append(rep.Warnings, "user "+u.AccessKey+" has MinIO policy "+u.PolicyName+"; DataSafe role is user")
		}
	}
	memberOf := map[string][]string{}
	for _, u := range snap.Users {
		for _, g := range u.MemberOf {
			memberOf[g] = append(memberOf[g], u.AccessKey)
		}
	}
	sort.Slice(snap.Groups, func(i, j int) bool { return snap.Groups[i].Name < snap.Groups[j].Name })
	for _, g := range snap.Groups {
		id, cerr := ds.createTeam(ctx, g.Name)
		if cerr != nil {
			rep.Errors = append(rep.Errors, "group "+g.Name+": "+cerr.Error())
			rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "group", Name: g.Name, Reason: cerr.Error()})
			continue
		}
		var ids []string
		members := g.Members
		if len(members) == 0 {
			members = memberOf[g.Name]
		}
		for _, name := range members {
			uid := users[name]
			if uid == "" {
				rep.Warnings = append(rep.Warnings, "group "+g.Name+" member "+name+" was not imported")
				continue
			}
			ids = append(ids, uid)
		}
		if ids == nil {
			ids = []string{}
		}
		if err := ds.putMembers(ctx, id, ids); err != nil {
			rep.Errors = append(rep.Errors, "group "+g.Name+" members: "+err.Error())
			continue
		}
		rep.GroupsTransferred++
	}
	prevKeys, keyWarn, err := readIssuedKeys(opt.KeysFile)
	if err != nil {
		return rep, err
	}
	if keyWarn != "" {
		rep.Warnings = append(rep.Warnings, keyWarn)
	}
	now := time.Now().UTC()
	userStmts, refusedUsers, rightWarnings := userStatements(snap)
	rep.Warnings = append(rep.Warnings, rightWarnings...)
	for _, u := range snap.Users {
		if refusedUsers[u.AccessKey] == "" {
			continue
		}
		rep.Warnings = append(rep.Warnings, "user "+u.AccessKey+" was not given a key: "+refusedUsers[u.AccessKey])
		rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "user", Name: u.AccessKey, Reason: refusedUsers[u.AccessKey]})
	}
	var issued []issuedKey
	keysByUser := map[string]issuedKey{}
	var keysWarn string
	persistKeys := func() error {
		if strings.TrimSpace(opt.KeysFile) == "" {
			return nil
		}
		warn, err := writeKeysFile(opt.KeysFile, prevKeys, issued)
		if err != nil {
			return err
		}
		if warn != "" && keysWarn == "" {
			keysWarn = warn
			rep.Warnings = append(rep.Warnings, warn)
		}
		return nil
	}
	userKeyID := map[string]string{}
	for _, u := range snap.Users {
		if users[u.AccessKey] == "" || !accountEnabled(u.Status) || refusedUsers[u.AccessKey] != "" {
			if users[u.AccessKey] != "" && !accountEnabled(u.Status) {
				rep.Warnings = append(rep.Warnings, "user "+u.AccessKey+" is disabled; no access key was created")
			}
			continue
		}
		row, err := issueKey(ctx, ds, users[u.AccessKey], u.AccessKey, passwords[u.AccessKey], kept, prevKeys, "user", "")
		if err != nil {
			rep.Errors = append(rep.Errors, "user "+u.AccessKey+" access key: "+err.Error())
			continue
		}
		row.Source = "user"
		row.Status = "enabled"
		issued = append(issued, row)
		keysByUser[u.AccessKey] = row
		userKeyID[u.AccessKey] = row.AccessKey
		if err := persistKeys(); err != nil {
			return rep, err
		}
	}
	saKeyID := map[string]string{}
	for _, sa := range snap.ServiceAccounts {
		if users[sa.Parent] == "" {
			rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "service-account", Name: sa.AccessKey, Reason: "parent user was not imported"})
			continue
		}
		if !accountEnabled(sa.Status) || (sa.Expires != nil && !sa.Expires.After(now)) || refusedUsers[sa.Parent] != "" || saInlineRefused(sa) {
			continue
		}
		if userKeyID[sa.Parent] == "" && !accountEnabled(userStatus(snap, sa.Parent)) {
			continue
		}
		exp := ""
		if sa.Expires != nil {
			exp = sa.Expires.UTC().Format(time.RFC3339)
		}
		row, err := issueKey(ctx, ds, users[sa.Parent], sa.Parent, passwords[sa.Parent], kept, prevKeys, sa.AccessKey, exp)
		if err != nil {
			rep.Errors = append(rep.Errors, "service account "+sa.AccessKey+": "+err.Error())
			continue
		}
		row.Source = sa.AccessKey
		row.Password = ""
		row.ExpiresAt = exp
		row.Status = "enabled"
		issued = append(issued, row)
		saKeyID[sa.AccessKey] = row.AccessKey
		if err := persistKeys(); err != nil {
			return rep, err
		}
	}
	for _, u := range snap.Users {
		if users[u.AccessKey] == "" || !accountEnabled(u.Status) || refusedUsers[u.AccessKey] != "" {
			continue
		}
		if _, ok := keysByUser[u.AccessKey]; ok {
			continue
		}
		rep.Errors = append(rep.Errors, "user "+u.AccessKey+" has no access key after import")
	}
	docs, principals, bindWarnings, groupNotes, saNotes, _ := EffectiveIAM(snap, userStmts, refusedUsers, userKeyID, saKeyID, now)
	rep.Warnings = append(rep.Warnings, bindWarnings...)
	rep.Groups = groupNotes
	rep.ServiceAccounts = saNotes
	mapped, mapWarnings := MapBucketPolicies(docs, principals)
	rep.Warnings = append(rep.Warnings, mapWarnings...)
	if err := persistKeys(); err != nil {
		return rep, err
	}
	absent := 0
	stored := map[string]struct{}{}
	for bucket, statements := range mapped {
		existing, found, err := ds.getPolicy(ctx, bucket)
		if err != nil {
			rep.Errors = append(rep.Errors, "policy "+bucket+": "+err.Error())
			continue
		}
		if !found {
			absent++
			rep.Warnings = append(rep.Warnings, "bucket "+bucket+" does not exist; its policy was not stored and access was not checked")
			rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "policy", Name: bucket, Reason: "bucket does not exist"})
			continue
		}
		merged, err := MergeBucketPolicy(existing, statements)
		if err != nil {
			rep.Errors = append(rep.Errors, "policy "+bucket+" was left unchanged: "+err.Error())
			continue
		}
		if err := policy.Validate(merged); err != nil {
			rep.Errors = append(rep.Errors, "policy "+bucket+" was left unchanged: "+err.Error())
			continue
		}
		if err := ds.putPolicy(ctx, bucket, merged); err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
				absent++
				rep.Warnings = append(rep.Warnings, "bucket "+bucket+" does not exist; its policy was not stored and access was not checked")
				rep.NotTransferred = append(rep.NotTransferred, IAMSkip{Kind: "policy", Name: bucket, Reason: "bucket does not exist"})
				continue
			}
			rep.Errors = append(rep.Errors, "policy "+bucket+": "+err.Error())
			continue
		}
		rep.PoliciesTransferred++
		stored[bucket] = struct{}{}
	}
	leader := ds.current()
	secureProbe := strings.HasPrefix(strings.ToLower(leader.URL), "https:")
	skippedList := 0
	for _, u := range snap.Users {
		row, ok := keysByUser[u.AccessKey]
		if !ok {
			continue
		}
		for bucket, statements := range mapped {
			run, want := listProbePlan(statements, policy.AccessKeyARN(row.AccessKey), bucket, secureProbe)
			if !run {
				continue
			}
			if _, ok := stored[bucket]; !ok {
				skippedList++
				continue
			}
			status, perr := probeList(ctx, leader, row, bucket)
			got := perr == nil && status >= 200 && status < 300
			verdict := "FAIL"
			if perr == nil && got == want {
				verdict = "PASS"
			}
			rep.Probes = append(rep.Probes, IAMProbe{
				Username: u.AccessKey, Bucket: bucket, Action: "s3:ListBucket", Status: status, Verdict: verdict,
			})
		}
	}
	sort.Slice(rep.Probes, func(i, j int) bool {
		if rep.Probes[i].Username != rep.Probes[j].Username {
			return rep.Probes[i].Username < rep.Probes[j].Username
		}
		return rep.Probes[i].Bucket < rep.Probes[j].Bucket
	})
	rep.Verdicts["users"] = passFail(rep.UsersTransferred == len(snap.Users) && len(snap.Users) > 0)
	if len(snap.Users) == 0 {
		rep.Verdicts["users"] = "PASS"
	}
	rep.Verdicts["groups"] = passFail(rep.GroupsTransferred == len(snap.Groups))
	rep.Verdicts["policies"] = passFail(rep.PoliciesTransferred+absent == len(mapped))
	accessPass := true
	for _, p := range rep.Probes {
		if p.Verdict != "PASS" {
			accessPass = false
		}
	}
	if len(rep.Probes) == 0 && skippedList == 0 {
		rep.Warnings = append(rep.Warnings, "no ListBucket request was sent; access was not checked")
		accessPass = false
	}
	if len(rep.Errors) > 0 {
		accessPass = false
	}
	rep.Verdicts["access"] = passFail(accessPass)
	if err := writeIAMReport(opt.ReportDir, rep); err != nil {
		return rep, err
	}
	fmt.Fprintf(opt.Stderr, "iam users %d groups %d policies %d report %s\n", rep.UsersTransferred, rep.GroupsTransferred, rep.PoliciesTransferred, opt.ReportDir)
	if len(rep.Errors) > 0 {
		return rep, fmt.Errorf("iam import recorded %d error(s)", len(rep.Errors))
	}
	return rep, nil
}

func userStatus(snap IAMSnapshot, name string) string {
	for _, u := range snap.Users {
		if u.AccessKey == name {
			return u.Status
		}
	}
	return ""
}

func saInlineRefused(sa IAMServiceAccount) bool {
	if sa.ImpliedPolicy || strings.TrimSpace(sa.Policy) == "" {
		return false
	}
	_, _, refuse := portIAMDocument(sa.Policy)
	return refuse
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func splitPolicies(name string) []string {
	var out []string
	for _, p := range strings.Split(name, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func groupsOf(u IAMUser, groups []IAMGroup) []IAMGroup {
	var out []IAMGroup
	for _, g := range groups {
		if sliceHas(u.MemberOf, g.Name) || sliceHas(g.Members, u.AccessKey) {
			out = append(out, g)
		}
	}
	return out
}

func sliceHas(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}

func issueKey(ctx context.Context, ds *dsClient, userID, username, password string, kept []keptKey, prev []issuedKey, source, expires string) (issuedKey, error) {
	for _, k := range kept {
		if !keptMatches(k, username, source) {
			continue
		}
		if err := ds.importKey(ctx, userID, k.AccessKey, k.SecretKey, expires); err != nil && !strings.Contains(err.Error(), "409") {
			return issuedKey{}, err
		}
		return issuedKey{Username: username, Password: password, AccessKey: k.AccessKey, SecretKey: k.SecretKey, ExpiresAt: expires}, nil
	}
	if source != "user" && len(kept) > 0 {
		return issuedKey{}, fmt.Errorf("keep file has no key for service account %s", source)
	}
	if len(kept) > 0 && source == "user" {
		return issuedKey{}, fmt.Errorf("keep file has no key for %s", username)
	}
	if row, ok := findIssued(prev, username, source); ok && row.SecretKey != "" {
		if err := ds.importKey(ctx, userID, row.AccessKey, row.SecretKey, firstNonEmpty(expires, row.ExpiresAt)); err != nil && !strings.Contains(err.Error(), "409") {
			return issuedKey{}, err
		}
		if password == "" {
			password = row.Password
		}
		if expires == "" {
			expires = row.ExpiresAt
		}
		return issuedKey{Username: username, Password: password, AccessKey: row.AccessKey, SecretKey: row.SecretKey, ExpiresAt: expires}, nil
	}
	access := "AKIA" + randomSecret()[:16]
	secret := randomSecret()
	if err := ds.importKey(ctx, userID, access, secret, expires); err != nil {
		return issuedKey{}, err
	}
	return issuedKey{Username: username, Password: password, AccessKey: access, SecretKey: secret, ExpiresAt: expires}, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func keptMatches(k keptKey, username, source string) bool {
	if k.Username != username {
		return false
	}
	if source == "user" {
		return k.ServiceAccount == "" || k.ServiceAccount == "user"
	}
	return k.ServiceAccount == source
}

func readKeptKeys(path string) ([]keptKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []keptKey
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("keep access keys: %w", err)
	}
	for _, row := range rows {
		if row.Username == "" || row.AccessKey == "" || row.SecretKey == "" {
			return nil, fmt.Errorf("keep access keys: username, access_key, and secret_key are required")
		}
	}
	return rows, nil
}

func readIssuedKeys(path string) ([]issuedKey, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, "", nil
	}
	var rows []issuedKey
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, "keys file " + path + " could not be read and was not overwritten", nil
	}
	return rows, "", nil
}

func findIssued(rows []issuedKey, username, source string) (issuedKey, bool) {
	for _, row := range rows {
		src := row.Source
		if src == "" {
			src = "user"
		}
		if row.Username == username && src == source && row.SecretKey != "" {
			return row, true
		}
	}
	return issuedKey{}, false
}

func mergeIssued(prev, fresh []issuedKey) []issuedKey {
	out := append([]issuedKey(nil), prev...)
	index := map[string]int{}
	for i, row := range out {
		index[issuedIdentity(row)] = i
	}
	for _, row := range fresh {
		id := issuedIdentity(row)
		at, ok := index[id]
		if !ok {
			index[id] = len(out)
			out = append(out, row)
			continue
		}
		kept := out[at]
		if kept.SecretKey != "" {
			row.SecretKey = kept.SecretKey
			row.AccessKey = kept.AccessKey
		}
		if kept.Password != "" && row.Password == "" {
			row.Password = kept.Password
		}
		if kept.ExpiresAt != "" && row.ExpiresAt == "" {
			row.ExpiresAt = kept.ExpiresAt
		}
		out[at] = row
	}
	return out
}

func issuedIdentity(row issuedKey) string {
	src := row.Source
	if src == "" {
		src = "user"
	}
	return row.Username + "\x00" + src
}

func writeKeysFile(path string, prev, fresh []issuedKey) (string, error) {
	if path == "" {
		return "", fmt.Errorf("keys file path is empty")
	}
	rows := mergeIssued(prev, fresh)
	warn := ""
	target := path
	if _, err := os.Stat(path); err == nil {
		if _, keyWarn, rerr := readIssuedKeys(path); rerr != nil {
			return "", rerr
		} else if keyWarn != "" {
			target = path + ".new"
			warn = "keys file " + path + " was not overwritten; new keys were written to " + target
		}
	}
	if err := writeKeysAtomic(target, rows); err != nil {
		return "", err
	}
	if extra := restrictKeysFile(target); extra != nil {
		if warn != "" {
			warn += "; "
		}
		warn += "keys file " + target + " is not restricted to the owner: " + extra.Error()
	}
	return warn, nil
}

func writeKeysAtomic(path string, rows []issuedKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func writeIAMReport(dir string, rep IAMReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# MinIO IAM import\n\n")
	fmt.Fprintf(&b, "- Users: %d (%s)\n", rep.UsersTransferred, rep.Verdicts["users"])
	fmt.Fprintf(&b, "- Groups: %d (%s)\n", rep.GroupsTransferred, rep.Verdicts["groups"])
	fmt.Fprintf(&b, "- Policies: %d (%s)\n", rep.PoliciesTransferred, rep.Verdicts["policies"])
	fmt.Fprintf(&b, "- Access: %s\n\n", rep.Verdicts["access"])
	if len(rep.Groups) > 0 {
		b.WriteString("## Groups\n\n")
		for _, g := range rep.Groups {
			fmt.Fprintf(&b, "- %s status %s policy %q applied %t members %d: %s\n", g.Name, g.Status, g.Policy, g.PolicyApplied, len(g.Members), g.Detail)
		}
		b.WriteString("\n")
	}
	if len(rep.ServiceAccounts) > 0 {
		b.WriteString("## Service accounts\n\n")
		for _, sa := range rep.ServiceAccounts {
			fmt.Fprintf(&b, "- %s parent %s status %s expiration %s implied %t inline %s: %s\n", sa.AccessKey, sa.Parent, sa.Status, sa.Expiration, sa.ImpliedPolicy, sa.Inline, sa.Detail)
		}
		b.WriteString("\n")
	}
	if len(rep.Warnings) > 0 {
		b.WriteString("## WARN\n\n")
		for _, w := range rep.Warnings {
			b.WriteString("- " + w + "\n")
		}
		b.WriteString("\n")
	}
	if len(rep.NotTransferred) > 0 {
		b.WriteString("## Not transferred\n\n")
		for _, s := range rep.NotTransferred {
			fmt.Fprintf(&b, "- %s %s: %s\n", s.Kind, s.Name, s.Reason)
		}
		b.WriteString("\n")
	}
	if len(rep.Probes) > 0 {
		b.WriteString("## Access checks\n\n")
		for _, p := range rep.Probes {
			fmt.Fprintf(&b, "- %s %s %s %s HTTP %d\n", p.Verdict, p.Username, p.Action, p.Bucket, p.Status)
		}
	}
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(b.String()), 0o644)
}

func probeList(ctx context.Context, leader Endpoint, key issuedKey, bucket string) (int, error) {
	ep := Endpoint{URL: leader.URL, AccessKey: key.AccessKey, SecretKey: key.SecretKey, Region: leader.Region}
	if ep.Region == "" {
		ep.Region = "us-east-1"
	}
	resp, err := ep.do(ctx, http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"list-type": {"2"}}, nil, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, nil
}

func randomSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type dsClient struct {
	targets []Endpoint
	idx     int
	user    string
	pass    string
	token   string
}

func (c *dsClient) current() Endpoint {
	if len(c.targets) == 0 {
		return Endpoint{}
	}
	return c.targets[c.idx%len(c.targets)]
}

func (c *dsClient) noteLeaderDown(ctx context.Context) {
	prev := c.idx
	c.pickLeader(ctx)
	if c.idx == prev && len(c.targets) > 1 {
		c.idx = (c.idx + 1) % len(c.targets)
	}
}

func (c *dsClient) pickLeader(ctx context.Context) {
	if len(c.targets) < 2 {
		return
	}
	for i, ep := range c.targets {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ep.URL, "/")+"/healthz", nil)
		if err != nil {
			continue
		}
		resp, err := ep.httpClient().Do(req)
		if err != nil {
			continue
		}
		var body struct {
			IsLeader *bool `json:"is_leader"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if body.IsLeader != nil && *body.IsLeader {
			c.idx = i
			return
		}
	}
}

func (c *dsClient) login(ctx context.Context) error {
	c.pickLeader(ctx)
	var resp struct {
		Token string `json:"token"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/v1/admin/login", map[string]string{
		"username": c.user, "password": c.pass,
	}, &resp, false); err != nil {
		return err
	}
	if resp.Token == "" {
		return fmt.Errorf("target login returned no token")
	}
	c.token = resp.Token
	return nil
}

func (c *dsClient) createUser(ctx context.Context, username, password, status, tenant string) (string, bool, error) {
	body := map[string]string{
		"username": username, "password": password, "role": "user", "status": status,
	}
	if tenant != "" {
		body["tenant_id"] = tenant
	}
	var resp struct {
		ID     string `json:"id"`
		UserID string `json:"user_id"`
		Error  string `json:"error"`
	}
	err := c.call(ctx, http.MethodPost, "/api/v1/users", body, &resp, true)
	if err == nil {
		if resp.ID != "" {
			return resp.ID, false, nil
		}
		return resp.UserID, false, nil
	}
	if !strings.Contains(err.Error(), "409") {
		return "", false, err
	}
	var list struct {
		Users []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}
	if lerr := c.call(ctx, http.MethodGet, "/api/v1/users", nil, &list, true); lerr != nil {
		return "", false, err
	}
	for _, u := range list.Users {
		if u.Username == username {
			return u.ID, true, nil
		}
	}
	return "", false, err
}

func (c *dsClient) createTeam(ctx context.Context, name string) (string, error) {
	var list struct {
		Teams []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"teams"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v1/teams", nil, &list, true); err == nil {
		for _, team := range list.Teams {
			if team.Name == name && team.ID != "" {
				return team.ID, nil
			}
		}
	}
	var resp struct {
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/v1/teams", map[string]string{"name": name}, &resp, true); err != nil {
		return "", err
	}
	if resp.Team.ID == "" {
		return "", fmt.Errorf("team id missing")
	}
	return resp.Team.ID, nil
}

func (c *dsClient) putMembers(ctx context.Context, id string, userIDs []string) error {
	return c.call(ctx, http.MethodPut, "/api/v1/teams/"+url.PathEscape(id)+"/members", map[string]any{"user_ids": userIDs}, nil, true)
}

func (c *dsClient) importKey(ctx context.Context, userID, access, secret, expires string) error {
	body := map[string]string{
		"access_key": access, "secret_key": secret, "label": "minio-import",
	}
	if strings.TrimSpace(expires) != "" {
		body["expires_at"] = expires
	}
	return c.call(ctx, http.MethodPost, "/api/v1/users/"+url.PathEscape(userID)+"/access-keys", body, nil, true)
}

func (c *dsClient) getPolicy(ctx context.Context, bucket string) (string, bool, error) {
	var resp struct {
		Policy string `json:"policy"`
	}
	err := c.call(ctx, http.MethodGet, "/api/v1/buckets/"+url.PathEscape(bucket)+"/policy", nil, &resp, true)
	if err == nil {
		return resp.Policy, true, nil
	}
	if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
		return "", false, nil
	}
	return "", false, err
}

func (c *dsClient) putPolicy(ctx context.Context, bucket, document string) error {
	return c.call(ctx, http.MethodPut, "/api/v1/buckets/"+url.PathEscape(bucket)+"/policy", map[string]string{"policy": document}, nil, true)
}

func (c *dsClient) call(ctx context.Context, method, path string, in any, out any, auth bool) error {
	token := ""
	if auth {
		token = c.token
	}
	deadline := time.Now().Add(35 * time.Second)
	var last error
	for attempt := 0; ; attempt++ {
		status, raw, err := c.do(ctx, method, path, in, token)
		retry := false
		if err != nil {
			last = err
			retry = true
		} else if status == http.StatusServiceUnavailable && isNotLeader(status, string(raw)) {
			last = ErrNotLeader
			retry = true
		} else if status >= 300 {
			return fmt.Errorf("http %d %s", status, strings.TrimSpace(string(raw)))
		} else {
			if out != nil && len(bytes.TrimSpace(raw)) > 0 {
				if err := json.Unmarshal(raw, out); err != nil {
					return err
				}
			}
			return nil
		}
		if !retry || time.Now().After(deadline) {
			break
		}
		c.noteLeaderDown(ctx)
		delay := 200 * time.Millisecond
		if attempt > 0 {
			delay = time.Duration(attempt+1) * 400 * time.Millisecond
		}
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		if !sleepIAM(ctx, delay) {
			return ctx.Err()
		}
	}
	if last == nil {
		last = ErrNotLeader
	}
	return last
}

func (c *dsClient) do(ctx context.Context, method, path string, in any, token string) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(raw)
	}
	ep := c.current()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(ep.URL, "/")+path, body)
	if err != nil {
		return 0, nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := ep.httpClient().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, raw, nil
}

func sleepIAM(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
