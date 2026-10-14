import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ApiError, api } from "@/lib/api";
import { Button } from "@/components/ui/button";
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

function errorText(err: Error): string {
  if (err instanceof ApiError && typeof err.body?.message === "string") return err.body.message;
  return err.message;
}

export function DestroyKey({ bucket, disabled }: { bucket: string; disabled?: boolean }) {
  const { t } = useTranslation("encryption");
  const queryClient = useQueryClient();
  const [explain, setExplain] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [name, setName] = useState("");
  const [mismatch, setMismatch] = useState(false);

  const destroy = useMutation({
    mutationFn: () => api.destroyBucketKey(bucket, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["bucket-encryption", bucket] });
      queryClient.invalidateQueries({ queryKey: ["buckets"] });
      toast.success(t("destroyed"));
      setConfirm(false);
      setName("");
    },
    onError: (err: Error) => toast.error(errorText(err)),
  });

  function submit() {
    if (name !== bucket) {
      setMismatch(true);
      return;
    }
    setMismatch(false);
    destroy.mutate();
  }

  return (
    <div className="space-y-2 rounded-md border border-destructive/40 p-3">
      <p className="text-sm font-medium text-destructive">{t("dangerZone")}</p>
      <Button
        size="sm"
        variant="destructive"
        data-testid="encryption-destroy"
        disabled={disabled || destroy.isPending}
        onClick={() => {
          setName("");
          setMismatch(false);
          setExplain(true);
        }}
      >
        {t("destroy")}
      </Button>
      <Dialog open={explain} onOpenChange={setExplain}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("destroyTitle")}</DialogTitle>
            <DialogDescription>{t("destroyBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExplain(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="destructive"
              data-testid="encryption-destroy-next"
              onClick={() => {
                setExplain(false);
                setConfirm(true);
              }}
            >
              {t("destroyNext")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={confirm}
        onOpenChange={(open) => {
          setConfirm(open);
          if (!open) setMismatch(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("destroyNameTitle")}</DialogTitle>
            <DialogDescription>{t("destroyNameHint", { bucket })}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1">
            <Label htmlFor="encryption-destroy-name">{bucket}</Label>
            <Input
              id="encryption-destroy-name"
              data-testid="encryption-destroy-name"
              value={name}
              autoComplete="off"
              onChange={(e) => {
                setName(e.target.value);
                setMismatch(false);
              }}
            />
            {mismatch && (
              <p className="text-sm text-destructive" data-testid="encryption-destroy-mismatch">
                {t("destroyMismatch")}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="destructive"
              data-testid="encryption-destroy-confirm"
              disabled={destroy.isPending}
              onClick={submit}
            >
              {t("destroyConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
