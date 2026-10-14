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

function errorText(err: Error): string {
  if (err instanceof ApiError && typeof err.body?.message === "string") return err.body.message;
  return err.message;
}

export function RotateKey({
  bucket,
  keyId,
  disabled,
}: {
  bucket: string;
  keyId: string;
  disabled?: boolean;
}) {
  const { t } = useTranslation("encryption");
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const rotate = useMutation({
    mutationFn: () => api.rotateKMSKey(keyId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["bucket-encryption", bucket] });
      toast.success(t("rotated"));
      setOpen(false);
    },
    onError: (err: Error) => toast.error(errorText(err)),
  });

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        data-testid="encryption-rotate"
        disabled={disabled || !keyId || rotate.isPending}
        onClick={() => setOpen(true)}
      >
        {t("rotate")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rotateTitle")}</DialogTitle>
            <DialogDescription>{t("rotateBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              data-testid="encryption-rotate-confirm"
              disabled={rotate.isPending}
              onClick={() => rotate.mutate()}
            >
              {t("rotateConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
