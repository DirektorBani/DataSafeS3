import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ApiError, api, type KMSSettings } from "@/lib/api";
import { Button } from "@datasafe/ui/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";

type TestResult = { ok: boolean; error?: string; message?: string };

function errorText(t: (key: string) => string, err: unknown): string {
  if (err instanceof ApiError) {
    const code = typeof err.body?.error === "string" ? err.body.error : "";
    const known = messageForCode(t, code);
    if (known) return known;
    if (typeof err.body?.message === "string" && err.body.message) return err.body.message;
  }
  return err instanceof Error ? err.message : t("error.unavailable");
}

function messageForCode(t: (key: string) => string, code: string): string {
  switch (code) {
    case "bad_secret":
      return t("error.badSecret");
    case "no_permission":
      return t("error.noPermission");
    case "timeout":
      return t("error.timeout");
    case "unavailable":
      return t("error.unavailable");
    case "not_configured":
      return t("error.notConfigured");
    default:
      return "";
  }
}

export function KMSSettingsPage() {
  const { t } = useTranslation("kms");
  const queryClient = useQueryClient();
  const settings = useQuery({
    queryKey: ["kms-settings"],
    queryFn: () => api.getKMSSettings(),
    refetchInterval: 5000,
  });
  const nodes = useQuery({
    queryKey: ["kms-nodes"],
    queryFn: () => api.getKMSNodes(),
    refetchInterval: 5000,
  });
  const view = settings.data;
  const [addr, setAddr] = useState("");
  const [auth, setAuth] = useState<"approle" | "token">("approle");
  const [roleID, setRoleID] = useState("");
  const [secretID, setSecretID] = useState("");
  const [token, setToken] = useState("");
  const [mount, setMount] = useState("transit");
  const [namespace, setNamespace] = useState("");
  const [ca, setCa] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [proven, setProven] = useState("");
  const hydrated = useRef(false);

  useEffect(() => {
    if (!view || hydrated.current) return;
    hydrated.current = true;
    setAddr(view.addr ?? "");
    setAuth(view.auth === "token" ? "token" : "approle");
    setRoleID(view.role_id ?? "");
    setMount(view.transit_mount || "transit");
    setNamespace(view.namespace ?? "");
    setCa(view.ca_cert ?? "");
    setSecretID("");
    setToken("");
  }, [view]);

  const locked = view?.env_locked ?? {};
  const body = () => ({
    addr: addr.trim(),
    namespace: namespace.trim(),
    transit_mount: mount.trim() || "transit",
    ca_cert: ca,
    auth,
    role_id: roleID.trim(),
    secret_id: secretID,
    token,
  });

  const formKey = JSON.stringify(body());
  const canSave = addr.trim() === "" || proven === formKey;
  const save = useMutation({
    mutationFn: () => api.putKMSSettings(body()),
    onSuccess: (data) => {
      queryClient.setQueryData(["kms-settings"], data);
      toast.success(t("saved"));
      setAddr(data.addr ?? "");
      setAuth(data.auth === "token" ? "token" : "approle");
      setRoleID(data.role_id ?? "");
      setMount(data.transit_mount || "transit");
      setNamespace(data.namespace ?? "");
      setCa(data.ca_cert ?? "");
      setSecretID("");
      setToken("");
      setProven("");
    },
    onError: (err: unknown) => toast.error(errorText(t, err)),
  });
  const test = useMutation({
    mutationFn: () => api.testKMS(body()),
    onSuccess: (data) => {
      setTestResult(data);
      setProven(JSON.stringify(body()));
    },
    onError: (err: unknown) => {
      const message = errorText(t, err);
      const code = err instanceof ApiError && typeof err.body?.error === "string" ? err.body.error : "";
      setTestResult({ ok: false, error: code, message });
      setProven("");
    },
  });

  if (settings.isLoading || !view) {
    return <p className="text-sm text-muted-foreground">{t("status.checking")}</p>;
  }

  const status = (view.status || "unconfigured") as KMSSettings["status"];
  const statusLabel = t(`status.${status}`, { defaultValue: t("status.unconfigured") });

  return (
    <div className="space-y-4">
      <Card data-testid="kms-settings">
        <CardHeader>
          <CardTitle className="text-base">{t("title")}</CardTitle>
          <CardDescription>{t("description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm" data-testid="kms-status">
            <span className="font-medium">{statusLabel}</span>
            {status === "error" && view.message ? <span className="text-destructive"> — {messageForCode(t, view.error || "") || view.message}</span> : null}
          </p>
          <div className="space-y-1">
            <Label htmlFor="kms-addr">{t("addr")}</Label>
            <Input
              id="kms-addr"
              data-testid="kms-addr"
              value={addr}
              disabled={!!locked.addr}
              onChange={(e) => setAddr(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("addrHint")}</p>
            {locked.addr && (
              <p className="text-xs text-muted-foreground" data-testid="kms-addr-locked">
                {t("envLocked")}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium">{t("auth")}</p>
            <p className="text-xs text-muted-foreground">{t("authHint")}</p>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="kms-auth"
                data-testid="kms-auth-approle"
                checked={auth === "approle"}
                disabled={!!locked.secret || !!locked.token}
                onChange={() => setAuth("approle")}
              />
              {t("approle")}
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="kms-auth"
                data-testid="kms-auth-token"
                checked={auth === "token"}
                disabled={!!locked.secret || !!locked.token}
                onChange={() => setAuth("token")}
              />
              {t("token")}
            </label>
          </div>
          {auth === "approle" ? (
            <>
              <div className="space-y-1">
                <Label htmlFor="kms-role">{t("roleId")}</Label>
                <Input
                  id="kms-role"
                  data-testid="kms-role-id"
                  value={roleID}
                  disabled={!!locked.role_id}
                  onChange={(e) => setRoleID(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">{t("roleIdHint")}</p>
              </div>
              <div className="space-y-1">
                <Label htmlFor="kms-secret">{t("secretId")}</Label>
                <Input
                  id="kms-secret"
                  data-testid="kms-secret"
                  type="password"
                  value={secretID}
                  placeholder={view.secret_id_set ? t("secretSet") : ""}
                  disabled={!!locked.secret}
                  onChange={(e) => setSecretID(e.target.value)}
                />
                {view.secret_id_set && (
                  <p className="text-xs text-muted-foreground" data-testid="kms-secret-set">
                    {t("secretSet")}
                  </p>
                )}
                <p className="text-xs text-muted-foreground">{t("secretIdHint")}</p>
                {locked.secret && <p className="text-xs text-muted-foreground">{t("envLocked")}</p>}
              </div>
            </>
          ) : (
            <div className="space-y-1">
              <Label htmlFor="kms-token">{t("token")}</Label>
              <Input
                id="kms-token"
                data-testid="kms-token"
                type="password"
                value={token}
                placeholder={view.token_set ? t("secretSet") : ""}
                disabled={!!locked.token}
                onChange={(e) => setToken(e.target.value)}
              />
              {view.token_set && (
                <p className="text-xs text-muted-foreground" data-testid="kms-token-set">
                  {t("secretSet")}
                </p>
              )}
              <p className="text-xs text-muted-foreground">{t("tokenHint")}</p>
            </div>
          )}
          <div className="space-y-1">
            <Label htmlFor="kms-mount">{t("mount")}</Label>
            <Input
              id="kms-mount"
              data-testid="kms-mount"
              value={mount}
              disabled={!!locked.mount}
              onChange={(e) => setMount(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("mountHint")}</p>
          </div>
          <details className="space-y-3">
            <summary className="cursor-pointer text-sm font-medium">{t("advanced")}</summary>
            <div className="space-y-1 pt-2">
              <Label htmlFor="kms-namespace">{t("namespace")}</Label>
              <Input
                id="kms-namespace"
                data-testid="kms-namespace"
                value={namespace}
                disabled={!!locked.namespace}
                onChange={(e) => setNamespace(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("namespaceHint")}</p>
            </div>
            <div className="space-y-1">
              <Label htmlFor="kms-ca">{t("ca")}</Label>
              <Input
                id="kms-ca"
                data-testid="kms-ca"
                value={ca}
                disabled={!!locked.ca_cert}
                onChange={(e) => setCa(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("caHint")}</p>
            </div>
          </details>
          {testResult && (
            <p
              className={`text-sm ${testResult.ok ? "text-foreground" : "text-destructive"}`}
              data-testid="kms-test-result"
            >
              {testResult.ok ? t("connected") : testResult.message}
            </p>
          )}
          {!canSave && <p className="text-xs text-muted-foreground">{t("saveNeedTest")}</p>}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              data-testid="kms-test"
              disabled={test.isPending}
              onClick={() => test.mutate()}
            >
              {test.isPending ? t("testing") : t("test")}
            </Button>
            <Button type="button" data-testid="kms-save" disabled={!canSave || save.isPending} onClick={() => save.mutate()}>
              {t("save")}
            </Button>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("nodesTitle")}</CardTitle>
          <CardDescription>{t("nodesHint")}</CardDescription>
        </CardHeader>
        <CardContent>
          {(nodes.data?.nodes.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">{t("nodesEmpty")}</p>
          ) : (
            <table className="w-full text-sm" data-testid="kms-nodes">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="py-2 pr-3 font-medium">{t("colNode")}</th>
                  <th className="py-2 pr-3 font-medium">{t("colStatus")}</th>
                  <th className="py-2 pr-3 font-medium">{t("colError")}</th>
                  <th className="py-2 font-medium">{t("colChecked")}</th>
                </tr>
              </thead>
              <tbody>
                {nodes.data?.nodes.map((node) => (
                  <tr key={node.node_id} className="border-b" data-testid={`kms-node-${node.node_id}`}>
                    <td className="py-2 pr-3">{node.node_id}</td>
                    <td className="py-2 pr-3">
                      {node.available === true ? t("nodeUp") : node.available === false ? t("nodeDown") : t("nodeUnknown")}
                    </td>
                    <td className="py-2 pr-3">{node.error ? messageForCode(t, node.error) || node.error : ""}</td>
                    <td className="py-2">{node.checked_at ?? ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
