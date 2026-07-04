import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Plus, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { api, type ConsoleUser, type Team } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";

export function TeamsPage() {
  const { t } = useTranslation("teams");
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [selected, setSelected] = useState<Team | null>(null);
  const [memberIds, setMemberIds] = useState<string[]>([]);

  const teams = useQuery({ queryKey: ["teams"], queryFn: () => api.listTeams() });
  const users = useQuery({ queryKey: ["users"], queryFn: () => api.listUsers() });
  const members = useQuery({
    queryKey: ["team-members", selected?.id],
    queryFn: () => api.listTeamMembers(selected!.id),
    enabled: !!selected?.id,
  });

  const createMutation = useMutation({
    mutationFn: () => api.createTeam(name.trim()),
    onSuccess: () => {
      setName("");
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      toast.success(t("toast.created"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteTeam(id),
    onSuccess: () => {
      setSelected(null);
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      toast.success(t("toast.deleted"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const saveMembersMutation = useMutation({
    mutationFn: () => api.setTeamMembers(selected!.id, memberIds),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["team-members", selected?.id] });
      toast.success(t("toast.membersSaved"));
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const selectTeam = (team: Team) => {
    setSelected(team);
    setMemberIds([]);
  };

  const toggleMember = (userId: string) => {
    setMemberIds((prev) =>
      prev.includes(userId) ? prev.filter((id) => id !== userId) : [...prev, userId]
    );
  };

  const loadMembersIntoSelection = () => {
    const ids = members.data?.members.map((m) => m.user_id) ?? [];
    setMemberIds(ids);
  };

  return (
    <div className="space-y-6">
      <PageHeader title={t("title")} description={t("subtitle")} />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-4 w-4" />
              {t("title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex gap-2">
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("namePlaceholder")}
              />
              <Button
                type="button"
                disabled={!name.trim() || createMutation.isPending}
                onClick={() => createMutation.mutate()}
              >
                <Plus className="mr-1 h-4 w-4" />
                {t("create")}
              </Button>
            </div>
            {(teams.data?.teams ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("empty")}</p>
            ) : (
              <ul className="space-y-2">
                {(teams.data?.teams ?? []).map((team) => (
                  <li
                    key={team.id}
                    className={`flex items-center justify-between rounded border p-2 ${
                      selected?.id === team.id ? "border-primary" : ""
                    }`}
                  >
                    <button type="button" className="text-left text-sm font-medium" onClick={() => selectTeam(team)}>
                      {team.name}
                    </button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      onClick={() => deleteMutation.mutate(team.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("members")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {!selected ? (
              <p className="text-sm text-muted-foreground">{t("selectTeam")}</p>
            ) : (
              <>
                <Button type="button" variant="outline" size="sm" onClick={loadMembersIntoSelection}>
                  {t("members")}
                </Button>
                <div className="max-h-64 space-y-2 overflow-y-auto">
                  {(users.data?.users ?? []).map((u: ConsoleUser) => (
                    <div key={u.id} className="flex items-center gap-2">
                      <input
                        id={`member-${u.id}`}
                        type="checkbox"
                        className="h-4 w-4"
                        checked={memberIds.includes(u.id)}
                        onChange={() => toggleMember(u.id)}
                      />
                      <Label htmlFor={`member-${u.id}`} className="text-sm font-normal">
                        {u.username}
                      </Label>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  disabled={saveMembersMutation.isPending}
                  onClick={() => saveMembersMutation.mutate()}
                >
                  {t("saveMembers")}
                </Button>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
