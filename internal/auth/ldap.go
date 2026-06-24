package auth

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type LDAPSettings struct {
	URL          string
	BindDN       string
	BindPassword string
	BaseDN       string
	GroupDN      string
	UserAttr     string
	GroupAttr    string
}

// ResolveLDAPURL rewrites localhost loopback hosts to host.docker.internal when running in a container.
func ResolveLDAPURL(url string) string {
	return DeriveInternalIssuer(url)
}

type LDAPUser struct {
	Username string
	Email    string
	Groups   []string
}

func TestLDAPConn(cfg LDAPSettings) error {
	conn, err := ldap.DialURL(cfg.URL)
	if err != nil {
		return err
	}
	defer conn.Close()
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return fmt.Errorf("bind failed: %w", err)
		}
	}
	return nil
}

func AuthenticateLDAP(cfg LDAPSettings, username, password string) (LDAPUser, error) {
	conn, err := ldap.DialURL(cfg.URL)
	if err != nil {
		return LDAPUser{}, err
	}
	defer conn.Close()

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return LDAPUser{}, fmt.Errorf("service bind failed: %w", err)
		}
	}

	userAttr := cfg.UserAttr
	if userAttr == "" {
		userAttr = "cn"
	}
	attrs := []string{userAttr, "mail", "dn"}
	if cfg.GroupAttr != "" {
		attrs = append(attrs, cfg.GroupAttr)
	}
	filter := fmt.Sprintf("(%s=%s)", userAttr, ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		attrs,
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return LDAPUser{}, err
	}
	if len(result.Entries) == 0 {
		return LDAPUser{}, ErrInvalidCredentials
	}
	entry := result.Entries[0]
	groups := ldapGroupsForEntry(conn, cfg, entry)
	if err := conn.Bind(entry.DN, password); err != nil {
		return LDAPUser{}, ErrInvalidCredentials
	}

	email := entry.GetAttributeValue("mail")
	if email == "" {
		email = username + "@ldap.local"
	}
	return LDAPUser{Username: username, Email: email, Groups: groups}, nil
}

func ldapGroupsForEntry(conn *ldap.Conn, cfg LDAPSettings, entry *ldap.Entry) []string {
	groups := []string{}
	if cfg.GroupAttr != "" {
		for _, v := range entry.GetAttributeValues(cfg.GroupAttr) {
			if cn := ldapGroupCN(v); cn != "" {
				groups = append(groups, cn)
			}
		}
	}
	if cfg.GroupDN != "" && len(groups) == 0 {
		gf := fmt.Sprintf("(member=%s)", ldap.EscapeFilter(entry.DN))
		gr, err := conn.Search(ldap.NewSearchRequest(
			cfg.GroupDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			gf, []string{"cn"}, nil,
		))
		if err == nil {
			for _, g := range gr.Entries {
				if cn := g.GetAttributeValue("cn"); cn != "" {
					groups = append(groups, cn)
				}
			}
		}
	}
	return groups
}

func ListLDAPUsers(cfg LDAPSettings, limit int) ([]LDAPUser, error) {
	conn, err := ldap.DialURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return nil, err
		}
	}
	userAttr := cfg.UserAttr
	if userAttr == "" {
		userAttr = "cn"
	}
	filter := fmt.Sprintf("(%s=*)", userAttr)
	if limit <= 0 {
		limit = 100
	}
	attrs := []string{userAttr, "mail", "dn"}
	if cfg.GroupAttr != "" {
		attrs = append(attrs, cfg.GroupAttr)
	}
	result, err := conn.Search(ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, limit, 0, false,
		filter,
		attrs,
		nil,
	))
	if err != nil {
		return nil, err
	}
	var out []LDAPUser
	for _, e := range result.Entries {
		u := e.GetAttributeValue(userAttr)
		if u == "" {
			continue
		}
		email := e.GetAttributeValue("mail")
		if email == "" {
			email = u + "@ldap.local"
		}
		groups := ldapGroupsForEntry(conn, cfg, e)
		out = append(out, LDAPUser{Username: u, Email: email, Groups: groups})
	}
	return out, nil
}

func MapLDAPRole(groups []string, groupRoleMap map[string]string, defaultRole string) string {
	for _, g := range groups {
		if role, ok := groupRoleMap[g]; ok && role != "" {
			return role
		}
	}
	if defaultRole != "" {
		return defaultRole
	}
	return RoleUser
}

func ldapGroupCN(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if i := strings.Index(strings.ToLower(value), "cn="); i >= 0 {
		rest := value[i+3:]
		if j := strings.Index(rest, ","); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return value
}
