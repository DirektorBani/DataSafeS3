import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Lock } from "lucide-react";
import { toast } from "sonner";
import { ApiError, api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export type EncryptionBadgeBucket = {
  default_encryption?: string;
  encryption_provider?: string;
  kms_key_status?: string;
  kms_master_key_id?: string;
};

export function BucketEncryptionBadge({
  bucket,
}: {
  bucket: EncryptionBadgeBucket;
}) {
  const { t } = useTranslation("encryption");
  if (bucket.kms_key_status === "destroyed") {
    return (
      <Badge variant="destructive" data-testid="bucket-encryption-badge" className="gap-1">
        <Lock className="h-3 w-3" />
        {t("badge.destroyed")}
      </Badge>
    );
  }
  if (!bucket.default_encryption) return null;
  if (bucket.encryption_provider === "vault-transit" || bucket.default_encryption === "aws:kms") {
    return (
      <Badge variant="secondary" data-testid="bucket-encryption-badge" className="gap-1">
        <Lock className="h-3 w-3" />
        {t("badge.vault", { name: bucket.kms_master_key_id || t("badge.vaultKey") })}
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" data-testid="bucket-encryption-badge" className="gap-1">
      <Lock className="h-3 w-3" />
      {t("badge.server")}
    </Badge>
  );
}

function errorText(err: Error): string {
  if (err instanceof ApiError && typeof err.body?.message === "string") return err.body.message;
  return err.message;
}

export function EncryptionCard({ bucket }: { bucket: string }) {
  const { t } = useTranslation("encryption");
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["bucket-encryption", bucket],
    queryFn: () => api.getBucketEncryption(bucket),
    enabled: !!bucket,
  });
  const view = query.data;
  const [enabled, setEnabled] = useState(false);
  const [provider, setProvider] = useState<"local" | "vault">("local");
  const [keyID, setKeyID] = useState("");
  const [confirmOff, setConfirmOff] = useState(false);

  useEffect(() => {
    if (!view) return;
    setEnabled(view.enabled);
    setProvider(view.algorithm === "aws:kms" || view.provider === "vault-transit" ? "vault" : "local");
    setKeyID(view.key_id && view.key_id !== view.default_key_id ? view.key_id : "");
  }, [view]);

  const save = useMutation({
    mutationFn: (body: { enabled: boolean; provider?: string; key_id?: string }) =>
      api.putBucketEncryption(bucket, body),
    onSuccess: (data) => {
      queryClient.setQueryData(["bucket-encryption", bucket], data);
      queryClient.invalidateQueries({ queryKey: ["buckets"] });
      toast.success(t("saved"));
      setConfirmOff(false);
    },
    onError: (err: Error) => toast.error(errorText(err)),
  });

  if (query.isLoading) {
    return <p className="text-sm text-muted-foreground">{t("loading")}</p>;
  }
  if (query.isError || !view) {
    return <p className="text-sm text-destructive">{t("loadError")}</p>;
  }

  const canEdit = view.can_edit;
  const keyReady = view.server_key_configured || view.vault_configured;
  const vaultSelected = provider === "vault";
  const publicRead = view.visibility === "public-read";

  return (
    <Card data-testid="encryption-card">
      <CardHeader>
        <CardTitle className="text-base">{t("title")}</CardTitle>
        <CardDescription>{t("description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!keyReady && <p className="text-sm text-muted-foreground">{t("serverKeyMissing")}</p>}
        {!canEdit && <p className="text-sm text-muted-foreground">{t("readOnly")}</p>}
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            className="mt-1"
            data-testid="encryption-toggle"
            checked={enabled}
            disabled={!canEdit || !keyReady || save.isPending}
            onChange={(e) => {
              if (!e.target.checked && view.enabled) {
                setConfirmOff(true);
                return;
              }
              setEnabled(e.target.checked);
            }}
          />
          <span>
            <span className="font-medium">{t("toggle")}</span>
            <span className="block text-xs text-muted-foreground">{t("newFilesOnly")}</span>
          </span>
        </label>
        {enabled && (
          <div className="space-y-3 pl-7">
            <label className="flex items-start gap-2 text-sm">
              <input
                type="radio"
                name="encryption-provider"
                data-testid="encryption-provider-local"
                checked={!vaultSelected}
                disabled={!canEdit || !view.server_key_configured}
                onChange={() => setProvider("local")}
              />
              <span>
                <span className="font-medium">{t("localTitle")}</span>
                <span className="block text-xs text-muted-foreground">{t("localHint")}</span>
              </span>
            </label>
            <label className="flex items-start gap-2 text-sm">
              <input
                type="radio"
                name="encryption-provider"
                data-testid="encryption-provider-vault"
                checked={vaultSelected}
                disabled
              />
              <span>
                <span className="font-medium text-muted-foreground">{t("vaultTitle")}</span>
                <span className="block text-xs text-muted-foreground">
                  {t("vaultHint")}{" "}
                  <Link to="/admin/settings/security" className="text-primary hover:underline">
                    {t("vaultLink")}
                  </Link>
                </span>
                {publicRead && (
                  <span className="mt-1 block text-xs text-destructive" data-testid="encryption-public-warning">
                    {t("publicKms")}
                  </span>
                )}
              </span>
            </label>
            <div className="space-y-1">
              <Label htmlFor="encryption-key-id">{t("keyId")}</Label>
              <Input
                id="encryption-key-id"
                data-testid="encryption-key-id"
                value={keyID}
                disabled={!canEdit || !view.server_key_configured}
                placeholder={view.default_key_id}
                onChange={(e) => setKeyID(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t("keyIdHint", { allowlist: view.allowlist })}</p>
            </div>
            {canEdit && view.server_key_configured && provider === "local" && (
              <Button
                size="sm"
                data-testid="encryption-save"
                disabled={save.isPending}
                onClick={() =>
                  save.mutate({
                    enabled: true,
                    provider: "local",
                    key_id: keyID.trim(),
                  })
                }
              >
                {t("save")}
              </Button>
            )}
          </div>
        )}
        {view.enabled && view.key_status === "destroyed" && (
          <p className="text-sm text-destructive">{t("badge.destroyed")}</p>
        )}
      </CardContent>
      <Dialog open={confirmOff} onOpenChange={setConfirmOff}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("disableTitle")}</DialogTitle>
            <DialogDescription>{t("disableBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOff(false)}>
              {t("cancel")}
            </Button>
            <Button
              data-testid="encryption-disable-confirm"
              disabled={save.isPending}
              onClick={() => save.mutate({ enabled: false })}
            >
              {t("disableConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
