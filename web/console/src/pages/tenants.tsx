import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { FolderOpen, Plus, Trash2, UserPlus, Users } from "lucide-react";
import { toast } from "sonner";
import { api, type Bucket, type TenantGroup, type TenantMember } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { useAuth } from "@/hooks/use-auth";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

function GroupMultiSelect({
  groups,
  value,
  onChange,
}: {
  groups: TenantGroup[];
  value: string[];
  onChange: (ids: string[]) => void;
}) {
  const { t } = useTranslation("tenants");
  const toggle = (id: string) => {
    onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id]);
  };
  if (groups.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("groups.empty")}</p>;
  }
  return (
    <div className="flex flex-wrap gap-2">
      {groups.map((g) => (
        <Button
          key={g.id}
          type="button"
          size="sm"
          variant={value.includes(g.id) ? "default" : "outline"}
          onClick={() => toggle(g.id)}
        >
          {g.name}
        </Button>
      ))}
    </div>
  );
}

export function TenantsPage() {
  const { t, i18n } = useTranslation(["tenants", "common"]);
  const TENANT_ROLES = [
    { value: "tenant_admin", label: t("common:roles.tenantAdmin") },
    { value: "member", label: t("common:roles.member") },
    { value: "viewer", label: t("common:roles.viewer") },
  ];
  const ASSIGNABLE_TENANT_ROLES = TENANT_ROLES.filter((r) => r.value !== "tenant_admin");
  const accessLevels = [
    { value: "read", label: t("tenants:access.readOnly") },
    { value: "read_write", label: t("tenants:access.readWrite") },
  ];
  const roleLabel = (role: string) => TENANT_ROLES.find((r) => r.value === role)?.label ?? role;
  const { isAdmin, canManageTenant } = useAuth();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [selectedTenant, setSelectedTenant] = useState<string>("");
  const [section, setSection] = useState<"members" | "groups">("members");
  const [addUserId, setAddUserId] = useState("");
  const [addRole, setAddRole] = useState("member");
  const [addGroupIds, setAddGroupIds] = useState<string[]>([]);
  const [newUsername, setNewUsername] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState("member");
  const [newGroupIds, setNewGroupIds] = useState<string[]>([]);
  const [groupName, setGroupName] = useState("");
  const [groupExternalName, setGroupExternalName] = useState("");
  const [groupDesc, setGroupDesc] = useState("");
  const [groupAccess, setGroupAccess] = useState<"read" | "read_write">("read_write");
  const [editingGroup, setEditingGroup] = useState<string>("");
  const [editGroupBuckets, setEditGroupBuckets] = useState<string[]>([]);
  const [groupBucketChoices, setGroupBucketChoices] = useState<Bucket[]>([]);

  const canManageSelected = !!selectedTenant && canManageTenant(selectedTenant);
  const assignableRoles = isAdmin ? TENANT_ROLES : ASSIGNABLE_TENANT_ROLES;

  const tenants = useQuery({
    queryKey: ["tenants"],
    queryFn: async () => (await api.listTenants()).tenants,
  });

  const users = useQuery({
    queryKey: ["users"],
    queryFn: async () => (await api.listUsers()).users,
    enabled: isAdmin || canManageSelected,
  });

  const members = useQuery({
    queryKey: ["tenant-members", selectedTenant],
    queryFn: async () => (await api.listTenantMembers(selectedTenant)).members,
    enabled: !!selectedTenant,
  });

  const groups = useQuery({
    queryKey: ["tenant-groups", selectedTenant],
    queryFn: async () => (await api.listTenantGroups(selectedTenant)).groups,
    enabled: !!selectedTenant && canManageSelected,
  });

  const tenantBuckets = useQuery({
    queryKey: ["tenant-buckets", selectedTenant],
    queryFn: async () => (await api.listTenantBuckets(selectedTenant)).buckets,
    enabled: !!selectedTenant && canManageSelected && section === "groups",
  });

  const tenantBucketOptions = tenantBuckets.data ?? [];

  const create = useMutation({
    mutationFn: () => api.createTenant(name),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tenants"] });
      setName("");
      setSelectedTenant(res.tenant.id);
      toast.success(t("tenants:toast.tenantCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const addMember = useMutation({
    mutationFn: () =>
      api.addTenantMember(selectedTenant, { user_id: addUserId, role: addRole, group_ids: addGroupIds }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tenant-members", selectedTenant] });
      setAddUserId("");
      setAddGroupIds([]);
      toast.success(t("tenants:toast.memberAdded"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const createTenantUser = useMutation({
    mutationFn: () =>
      api.createTenantUser(selectedTenant, {
        username: newUsername,
        password: newPassword,
        email: newEmail || undefined,
        role: newRole as "member" | "viewer",
        group_ids: newGroupIds,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tenant-members", selectedTenant] });
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setNewUsername("");
      setNewEmail("");
      setNewPassword("");
      setNewRole("member");
      setNewGroupIds([]);
      toast.success(t("tenants:toast.userCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const createGroup = useMutation({
    mutationFn: () =>
      api.createTenantGroup(selectedTenant, {
        name: groupName,
        external_name: groupExternalName || undefined,
        description: groupDesc || undefined,
        access_level: groupAccess,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tenant-groups", selectedTenant] });
      setGroupName("");
      setGroupExternalName("");
      setGroupDesc("");
      setGroupAccess("read_write");
      toast.success(t("tenants:toast.groupCreated"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const updateRole = async (m: TenantMember, role: string) => {
    try {
      await api.updateTenantMember(selectedTenant, m.user_id, role);
      queryClient.invalidateQueries({ queryKey: ["tenant-members", selectedTenant] });
      toast.success(t("tenants:toast.roleUpdated"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const updateMemberGroups = async (userId: string, groupIds: string[]) => {
    try {
      await api.setMemberGroups(selectedTenant, userId, groupIds);
      queryClient.invalidateQueries({ queryKey: ["tenant-members", selectedTenant] });
      toast.success(t("tenants:toast.groupsUpdated"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const openGroupBuckets = async (g: TenantGroup) => {
    setSection("groups");
    setEditingGroup(g.id);
    try {
      const [detail, bucketsResp] = await Promise.all([
        api.getTenantGroup(selectedTenant, g.id),
        api.listTenantBuckets(selectedTenant),
      ]);
      const options = bucketsResp.buckets ?? [];
      setGroupBucketChoices(options);
      const keySet = new Set(detail.bucket_keys ?? []);
      setEditGroupBuckets(
        options.filter((b) => keySet.has(b.storage_key ?? b.name)).map((b) => b.storage_key ?? b.name)
      );
    } catch {
      setEditGroupBuckets([]);
    }
  };

  const saveGroupBuckets = async () => {
    if (!editingGroup) return;
    try {
      await api.setTenantGroupBuckets(selectedTenant, editingGroup, editGroupBuckets);
      queryClient.invalidateQueries({ queryKey: ["tenant-groups", selectedTenant] });
      setEditingGroup("");
      toast.success(t("tenants:toast.groupBucketsUpdated"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <div>
      <PageHeader
        title={t("tenants:title")}
        description={
          isAdmin
            ? t("tenants:description.admin")
            : t("tenants:description.tenantAdmin")
        }
      />
      {isAdmin && (
        <Card className="mb-6">
          <CardContent className="flex gap-3 pt-6 max-w-md">
            <div className="flex-1">
              <Label>{t("tenants:fields.tenantName")}</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <Button className="mt-6" onClick={() => create.mutate()} disabled={!name || create.isPending}>
              <Plus className="h-4 w-4" /> {t("tenants:actions.create")}
            </Button>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <div>
          {tenants.data?.map((tenant) => (
            <Card
              key={tenant.id}
              className={`mb-2 cursor-pointer ${selectedTenant === tenant.id ? "ring-2 ring-primary" : ""}`}
              onClick={() => setSelectedTenant(tenant.id)}
            >
              <CardContent className="flex justify-between py-4">
                <div>
                  <p className="font-medium">{tenant.name}</p>
                  <p className="text-xs text-muted-foreground">{tenant.id} — {tenant.status} — {formatDate(tenant.created_at)}</p>
                </div>
                {isAdmin && tenant.id !== "default" && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={async (e) => {
                      e.stopPropagation();
                      await api.deleteTenant(tenant.id);
                      queryClient.invalidateQueries({ queryKey: ["tenants"] });
                      if (selectedTenant === tenant.id) setSelectedTenant("");
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
              </CardContent>
            </Card>
          ))}
          {!tenants.isLoading && (tenants.data ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">{t("empty")}</p>
          )}
        </div>

        {selectedTenant && canManageSelected && (
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between gap-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  {section === "members" ? (
                    <><Users className="h-4 w-4" /> {t("tenants:sections.members")}</>
                  ) : (
                    <><FolderOpen className="h-4 w-4" /> {t("tenants:sections.groups")}</>
                  )}
                </CardTitle>
                <div className="flex gap-1">
                  <Button size="sm" variant={section === "members" ? "default" : "outline"} onClick={() => setSection("members")}>
                    {t("tenants:sections.members")}
                  </Button>
                  <Button size="sm" variant={section === "groups" ? "default" : "outline"} onClick={() => setSection("groups")}>
                    {t("tenants:sections.groups")}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {section === "groups" && (
                <>
                  <div className="rounded border p-3 space-y-3">
                    <p className="text-sm font-medium">{t("groups.create")}</p>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <div>
                        <Label>{t("tenants:fields.name")}</Label>
                        <Input value={groupName} onChange={(e) => setGroupName(e.target.value)} />
                      </div>
                      <div>
                        <Label>{t("tenants:fields.accessLevel")}</Label>
                        <Select value={groupAccess} onValueChange={(v) => setGroupAccess(v as "read" | "read_write")}>
                          <SelectTrigger><SelectValue /></SelectTrigger>
                          <SelectContent>
                            {accessLevels.map((a) => (
                              <SelectItem key={a.value} value={a.value}>{a.label}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="sm:col-span-2">
                        <Label>{t("tenants:fields.idpGroup")}</Label>
                        <Input
                          value={groupExternalName}
                          onChange={(e) => setGroupExternalName(e.target.value)}
                          placeholder={t("tenants:placeholder.idpGroup")}
                        />
                      </div>
                      <div className="sm:col-span-2">
                        <Label>{t("tenants:fields.description")}</Label>
                        <Input value={groupDesc} onChange={(e) => setGroupDesc(e.target.value)} />
                      </div>
                    </div>
                    <Button onClick={() => createGroup.mutate()} disabled={!groupName || createGroup.isPending}>
                      {t("groups.createAction")}
                    </Button>
                  </div>

                  {(groups.data ?? []).map((g) => (
                    <div key={g.id} className="rounded border p-3 text-sm space-y-2">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="font-medium">{g.name}</p>
                          <p className="text-xs text-muted-foreground">
                            {t("tenants:groups.summary", {
                              access: g.access_level === "read_write"
                                ? t("tenants:access.readWrite")
                                : t("tenants:access.readOnly"),
                              buckets: g.bucket_count ?? 0,
                              members: g.member_count ?? 0,
                            })}
                          </p>
                          {g.external_name && (
                            <p className="text-xs text-muted-foreground">{t("tenants:groups.idp")} {g.external_name}</p>
                          )}
                          {g.description && <p className="text-xs text-muted-foreground mt-1">{g.description}</p>}
                        </div>
                        <div className="flex gap-1">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={async () => {
                              const next = window.prompt(t("tenants:prompt.idpGroup"), g.external_name ?? "");
                              if (next === null) return;
                              try {
                                await api.updateTenantGroup(selectedTenant, g.id, { external_name: next });
                                queryClient.invalidateQueries({ queryKey: ["tenant-groups", selectedTenant] });
                                toast.success(t("tenants:toast.idpMappingUpdated"));
                              } catch (e) {
                                toast.error((e as Error).message);
                              }
                            }}
                          >
                            {t("tenants:groups.idpMap")}
                          </Button>
                          <Button size="sm" variant="outline" onClick={() => openGroupBuckets(g)}>
                            {t("tenants:groups.buckets")}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={async () => {
                              await api.deleteTenantGroup(selectedTenant, g.id);
                              queryClient.invalidateQueries({ queryKey: ["tenant-groups", selectedTenant] });
                              if (editingGroup === g.id) setEditingGroup("");
                            }}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                      {editingGroup === g.id && (
                        <div className="space-y-2 pt-2 border-t">
                          <Label>{t("tenants:groups.tenantBuckets")}</Label>
                          <div className="flex flex-wrap gap-2">
                            {(groupBucketChoices.length ? groupBucketChoices : tenantBucketOptions).map((b) => {
                              const key = b.storage_key ?? b.name;
                              const selected = editGroupBuckets.includes(key);
                              return (
                                <Button
                                  key={key}
                                  type="button"
                                  size="sm"
                                  variant={selected ? "default" : "outline"}
                                  onClick={() =>
                                    setEditGroupBuckets(
                                      selected ? editGroupBuckets.filter((k) => k !== key) : [...editGroupBuckets, key]
                                    )
                                  }
                                >
                                  {b.name}
                                </Button>
                              );
                            })}
                          </div>
                          {(groupBucketChoices.length ? groupBucketChoices : tenantBucketOptions).length === 0 && (
                            <p className="text-xs text-muted-foreground">{t("groups.noBuckets")}</p>
                          )}
                          <Button size="sm" onClick={saveGroupBuckets}>{t("groups.saveBuckets")}</Button>
                        </div>
                      )}
                    </div>
                  ))}
                  {!groups.isLoading && (groups.data ?? []).length === 0 && (
                    <p className="text-sm text-muted-foreground">{t("groups.emptyList")}</p>
                  )}
                </>
              )}

              {section === "members" && (
                <>
                  <div className="rounded border p-3 space-y-3">
                    <p className="text-sm font-medium flex items-center gap-2">
                      <UserPlus className="h-4 w-4" /> {t("tenants:users.create")}
                    </p>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <div>
                        <Label>{t("tenants:fields.username")}</Label>
                        <Input value={newUsername} onChange={(e) => setNewUsername(e.target.value)} />
                      </div>
                      <div>
                        <Label>{t("tenants:fields.email")}</Label>
                        <Input value={newEmail} onChange={(e) => setNewEmail(e.target.value)} />
                      </div>
                      <div>
                        <Label>{t("tenants:fields.password")}</Label>
                        <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
                      </div>
                      <div>
                        <Label>{t("tenants:fields.tenantRole")}</Label>
                        <Select value={newRole} onValueChange={setNewRole}>
                          <SelectTrigger><SelectValue /></SelectTrigger>
                          <SelectContent>
                            {ASSIGNABLE_TENANT_ROLES.map((r) => (
                              <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                    <div>
                      <Label className="mb-2 block">{t("tenants:fields.groups")}</Label>
                      <GroupMultiSelect groups={groups.data ?? []} value={newGroupIds} onChange={setNewGroupIds} />
                    </div>
                    <Button
                      onClick={() => createTenantUser.mutate()}
                      disabled={!newUsername || !newPassword || createTenantUser.isPending}
                    >
                      {t("tenants:users.createAction")}
                    </Button>
                  </div>

                  <div className="space-y-2">
                    <div className="flex gap-2">
                      <Select value={addUserId} onValueChange={setAddUserId}>
                        <SelectTrigger className="flex-1"><SelectValue placeholder={t("tenants:users.selectExisting")} /></SelectTrigger>
                        <SelectContent>
                          {(users.data ?? []).map((u) => (
                            <SelectItem key={u.id} value={u.id}>{u.username} ({u.email})</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Select value={addRole} onValueChange={setAddRole}>
                        <SelectTrigger className="w-44"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {assignableRoles.map((r) => (
                            <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button onClick={() => addMember.mutate()} disabled={!addUserId || addMember.isPending}>{t("tenants:users.add")}</Button>
                    </div>
                    <div>
                      <Label className="mb-2 block text-xs">{t("tenants:users.groupsForMember")}</Label>
                      <GroupMultiSelect groups={groups.data ?? []} value={addGroupIds} onChange={setAddGroupIds} />
                    </div>
                  </div>

                  {(members.data ?? []).map((m) => (
                    <div key={m.user_id} className="rounded border p-3 text-sm space-y-2">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="font-medium flex items-center gap-2 flex-wrap">
                            {m.username ?? m.user_id}
                            {m.role === "tenant_admin" && (
                              <Badge variant="secondary">{t("tenants:badge.tenantAdmin")}</Badge>
                            )}
                            {(m.groups ?? []).map((g) => (
                              <Badge key={g.id} variant="outline">{g.name}</Badge>
                            ))}
                          </p>
                          <p className="text-xs text-muted-foreground">{m.email}</p>
                        </div>
                        <div className="flex items-center gap-2">
                          <Select value={m.role} onValueChange={(v) => updateRole(m, v)}>
                            <SelectTrigger className="w-44 h-8"><SelectValue>{roleLabel(m.role)}</SelectValue></SelectTrigger>
                            <SelectContent>
                              {assignableRoles.map((r) => (
                                <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
                              ))}
                              {!isAdmin && m.role === "tenant_admin" && (
                                <SelectItem value="tenant_admin">{roleLabel("tenant_admin")}</SelectItem>
                              )}
                            </SelectContent>
                          </Select>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={async () => {
                              await api.removeTenantMember(selectedTenant, m.user_id);
                              queryClient.invalidateQueries({ queryKey: ["tenant-members", selectedTenant] });
                            }}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                      {m.role !== "tenant_admin" && (groups.data ?? []).length > 0 && (
                        <div>
                          <Label className="text-xs mb-1 block">{t("tenants:fields.groups")}</Label>
                          <GroupMultiSelect
                            groups={groups.data ?? []}
                            value={(m.groups ?? []).map((g) => g.id)}
                            onChange={(ids) => updateMemberGroups(m.user_id, ids)}
                          />
                        </div>
                      )}
                    </div>
                  ))}
                  {!members.isLoading && (members.data ?? []).length === 0 && (
                    <p className="text-sm text-muted-foreground">{t("members.empty")}</p>
                  )}
                </>
              )}
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
