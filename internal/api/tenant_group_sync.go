package api

import (
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// syncUserTenantGroupsFromExternal matches IdP/LDAP group names to tenant groups by
// external_name (if set) or display name within each tenant the user belongs to.
func (s *Server) syncUserTenantGroupsFromExternal(userID string, externalGroups []string) {
	if len(externalGroups) == 0 {
		return
	}
	ext := make(map[string]struct{}, len(externalGroups))
	for _, g := range externalGroups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		ext[strings.ToLower(g)] = struct{}{}
	}
	if len(ext) == 0 {
		return
	}
	memberships, _ := s.meta.ListUserTenants(userID)
	for _, m := range memberships {
		groups, err := s.meta.ListTenantGroups(m.TenantID)
		if err != nil || len(groups) == 0 {
			continue
		}
		var matched []string
		for _, g := range groups {
			if tenantGroupMatchesExternal(g, ext) {
				matched = append(matched, g.ID)
			}
		}
		if len(matched) == 0 {
			continue
		}
		existing, _ := s.meta.ListUserTenantGroupIDs(m.TenantID, userID)
		seen := make(map[string]struct{}, len(existing)+len(matched))
		merged := make([]string, 0, len(existing)+len(matched))
		for _, id := range existing {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
		for _, id := range matched {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
		_ = s.meta.ReplaceUserTenantGroups(m.TenantID, userID, merged)
	}
}

func tenantGroupMatchesExternal(g metadata.TenantGroupRecord, ext map[string]struct{}) bool {
	if _, ok := ext[strings.ToLower(g.Name)]; ok {
		return true
	}
	if en := strings.TrimSpace(g.ExternalName); en != "" {
		if _, ok := ext[strings.ToLower(en)]; ok {
			return true
		}
	}
	return false
}
