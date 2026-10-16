package auth

import "strings"

// Access-model cells. The same table is copied into the operations guide.
// allow: the role may do it. deny: it may not. write-grant: only with a write
// grant on that bucket. compat-flag: only when STORAGE_OPERATOR_WRITE_ALL=true.
const (
	cellAllow = "allow"
	cellDeny  = "deny"
	cellGrant = "write-grant"
	cellFlag  = "compat-flag"
)

type accessModelRow struct {
	Action      string
	GlobalAdmin string
	Operator    string
	TenantAdmin string
	BucketAdmin string
	Write       string
	Read        string
	Anonymous   string
}

func accessModelRows() []accessModelRow {
	return []accessModelRow{
		{"presign GET", cellAllow, cellAllow, cellAllow, cellAllow, cellAllow, cellAllow, cellDeny},
		{"presign PUT", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"presign DELETE", cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
		{"create public-read bucket", cellAllow, cellDeny, cellAllow, cellDeny, cellDeny, cellDeny, cellDeny},
		{"set public-read after create", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"anonymous GET when public and not blocked", cellAllow, cellAllow, cellAllow, cellAllow, cellAllow, cellAllow, cellAllow},
		{"anonymous GET when block public access", cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
		{"list own share links", cellAllow, cellAllow, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"list every share link", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"create share link", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"revoke own share link", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"revoke another user's share link", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"revoke all share links", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"change bucket sharing policy", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"change tenant sharing policy", cellAllow, cellDeny, cellAllow, cellDeny, cellDeny, cellDeny, cellDeny},
		{"change global sharing policy", cellAllow, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
		{"legal hold", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"retention GOVERNANCE", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"retention COMPLIANCE", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"change storage class", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"restore from trash", cellAllow, cellGrant, cellAllow, cellAllow, cellAllow, cellDeny, cellDeny},
		{"purge trash", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"change bucket grants", cellAllow, cellDeny, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"delete bucket", cellAllow, cellFlag, cellAllow, cellAllow, cellDeny, cellDeny, cellDeny},
		{"operator write without a grant", cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
		{"cluster rotate or revoke-notify without a peer certificate", cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
		{"STS RoleArn echoed unchecked", cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny, cellDeny},
	}
}

// AccessModelMarkdown is the canonical role matrix embedded in the guides.
func AccessModelMarkdown() string {
	var b strings.Builder
	b.WriteString("| Action | Global admin | Operator | Tenant admin | Bucket admin | Write | Read | Anonymous |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range accessModelRows() {
		b.WriteString("| ")
		b.WriteString(row.Action)
		b.WriteString(" | ")
		b.WriteString(row.GlobalAdmin)
		b.WriteString(" | ")
		b.WriteString(row.Operator)
		b.WriteString(" | ")
		b.WriteString(row.TenantAdmin)
		b.WriteString(" | ")
		b.WriteString(row.BucketAdmin)
		b.WriteString(" | ")
		b.WriteString(row.Write)
		b.WriteString(" | ")
		b.WriteString(row.Read)
		b.WriteString(" | ")
		b.WriteString(row.Anonymous)
		b.WriteString(" |\n")
	}
	return b.String()
}
