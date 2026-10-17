// Package contract is the Desktop compatibility suite (spec 4.6.6 and 4.6.8).
// The agent feature columns in cmd/datasafe-agent/compat.go are checked
// against Matrix. Live probes run only when DATASAFE_CONTRACT_URL is set.
package contract

// Feature is one row of spec table 4.6.6. The first table row (password,
// TOTP, and WebAuthn login) is auth.password and auth.totp. WebAuthn login
// does not have its own feature key.
type Feature struct {
	Name  string
	In150 bool
	In160 bool
}

// Matrix is table 4.6.6. After A0 the same Name is the capabilities key.
// A key that is off in both columns arrives only in an authenticated
// features object (auth.refresh, objects.list_parts, objects.range,
// errors.codes, server.secrets_stdin, server.schema_version).
var Matrix = []Feature{
	{Name: "auth.password", In150: true, In160: true},
	{Name: "auth.totp", In150: true, In160: true},
	{Name: "auth.oidc", In150: true, In160: true},
	{Name: "auth.refresh", In150: false, In160: false},
	{Name: "objects.multipart", In150: true, In160: true},
	{Name: "objects.list_parts", In150: false, In160: false},
	{Name: "objects.range", In150: false, In160: false},
	{Name: "sharing.links", In150: true, In160: true},
	{Name: "sharing.policy", In150: false, In160: true},
	{Name: "sharing.revoke_all", In150: false, In160: true},
	{Name: "auth.step_up", In150: false, In160: true},
	{Name: "encryption.kms", In150: false, In160: true},
	{Name: "errors.codes", In150: false, In160: false},
	{Name: "server.secrets_stdin", In150: false, In160: false},
	{Name: "server.schema_version", In150: false, In160: false},
}

// Column is the feature map for a stamped profile, "1.5.0" or "1.6.0".
func Column(profile string) map[string]int {
	out := map[string]int{}
	for _, row := range Matrix {
		on := false
		switch profile {
		case "1.5.0":
			on = row.In150
		case "1.6.0":
			on = row.In160
		}
		if on {
			out[row.Name] = 1
		}
	}
	return out
}
