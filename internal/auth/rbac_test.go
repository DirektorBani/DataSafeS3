package auth_test

import "testing"

import "github.com/DirektorBani/datasafe/internal/auth"

func TestRBACHelpers(t *testing.T) {
	if !auth.CanManageUsers(auth.RoleAdministrator) {
		t.Fatal("admin should manage users")
	}
	if auth.CanManageUsers(auth.RoleOperator) {
		t.Fatal("operator should not manage users")
	}
	if !auth.CanSeeAllBuckets(auth.RoleOperator) {
		t.Fatal("operator sees all buckets")
	}
	if auth.CanSeeAllBuckets(auth.RoleUser) {
		t.Fatal("user should not see all buckets")
	}
	if !auth.CanAccessFederation(auth.RoleAdministrator) {
		t.Fatal("admin should access federation")
	}
	if auth.CanAccessFederation(auth.RoleOperator) {
		t.Fatal("operator should not access federation")
	}
	if auth.CanAccessFederation(auth.RoleUser) {
		t.Fatal("user should not access federation")
	}
	if !auth.CanAccessCluster(auth.RoleAdministrator) {
		t.Fatal("admin should access cluster")
	}
	if auth.CanAccessCluster(auth.RoleUser) {
		t.Fatal("user should not access cluster")
	}
	if !auth.CanAccessBucket(auth.RoleUser, "u1", "alice", []string{"t1"}, "u1", "alice", "", "", nil) {
		t.Fatal("user should access own bucket by owner_id")
	}
	if !auth.CanAccessBucket(auth.RoleUser, "u1", "alice", []string{"t1"}, "", "", "t1", "", nil) {
		t.Fatal("user should access team bucket")
	}
	if auth.CanAccessBucket(auth.RoleUser, "u1", "alice", []string{"t1"}, "u2", "bob", "t2", "", nil) {
		t.Fatal("user should not access unrelated bucket")
	}
	if auth.CanAccessBucket(auth.RoleUser, "u1", "alice", nil, "", "admin", "", "", nil) {
		t.Fatal("user should not access admin-owned legacy bucket")
	}
	if auth.CanAccessBucket(auth.RoleUser, "u1", "alice", nil, "", "", "", "", nil) {
		t.Fatal("user should not access orphan bucket without owner")
	}
	if !auth.CanAccessBucket(auth.RoleAdministrator, "u1", "alice", nil, "", "admin", "", "", nil) {
		t.Fatal("administrator should access any bucket")
	}
	if !auth.CanAccessBucket(auth.RoleUser, "u1", "alice", nil, "", "", "", "tenant-1", []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleViewer}}) {
		t.Fatal("tenant viewer should read tenant bucket")
	}
	if auth.CanWriteBucket(auth.RoleUser, "u1", "alice", nil, "", "", "", "tenant-1", []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleViewer}}) {
		t.Fatal("tenant viewer should not write tenant bucket")
	}
	if !auth.CanWriteBucket(auth.RoleUser, "u1", "alice", nil, "", "", "", "tenant-1", []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleMember}}) {
		t.Fatal("tenant member should write tenant bucket")
	}
	if !auth.CanWriteBucket(auth.RoleUser, "u1", "alice", nil, "", "", "", "tenant-1", []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleAdmin}}) {
		t.Fatal("tenant admin should write tenant bucket")
	}
	if !auth.CanManageTenant(auth.RoleUser, []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleAdmin}}, "tenant-1") {
		t.Fatal("tenant admin should manage tenant-1")
	}
	if auth.CanManageTenant(auth.RoleUser, []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleMember}}, "tenant-1") {
		t.Fatal("tenant member should not manage tenant")
	}
	if auth.CanManageTenant(auth.RoleUser, []auth.TenantMembership{{TenantID: "tenant-1", Role: auth.TenantRoleAdmin}}, "tenant-2") {
		t.Fatal("tenant admin should not manage other tenant")
	}
	if !auth.CanManageTenant(auth.RoleAdministrator, nil, "tenant-1") {
		t.Fatal("global admin should manage any tenant")
	}
	if auth.CanManageAnyTenant(auth.RoleUser, []auth.TenantMembership{{TenantID: "t1", Role: auth.TenantRoleMember}}) {
		t.Fatal("member should not manage any tenant")
	}
	if !auth.CanManageAnyTenant(auth.RoleUser, []auth.TenantMembership{{TenantID: "t1", Role: auth.TenantRoleAdmin}}) {
		t.Fatal("tenant admin should manage some tenant")
	}
	if !auth.CanAssignTenantRole(auth.RoleUser, auth.TenantRoleMember) {
		t.Fatal("tenant admin should assign member")
	}
	if auth.CanAssignTenantRole(auth.RoleUser, auth.TenantRoleAdmin) {
		t.Fatal("tenant admin should not assign tenant_admin")
	}
	if !auth.CanAssignTenantRole(auth.RoleAdministrator, auth.TenantRoleAdmin) {
		t.Fatal("global admin should assign tenant_admin")
	}
}
