import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  AlertTriangle,
  Ban,
  Clock,
  Database,
  Download,
  FileIcon,
  Loader2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatBytes, formatDate } from "@/lib/utils";

type ShareInfo = {
  filename: string;
  key: string;
  size: number;
  content_type: string;
  expires_at?: string;
  max_downloads: number;
  download_count: number;
  password_required?: boolean;
};

type ShareErrorKind =
  | "not_found"
  | "expired"
  | "limit_reached"
  | "blocked"
  | "invalid_password"
  | "unavailable"
  | "destroyed"
  | "generic";

type ShareError = {
  kind: ShareErrorKind;
  message: string;
};

function classifyShareError(
  status: number,
  message: string,
  t: (key: string) => string
): ShareError {
  const lower = message.toLowerCase();
  if (message.includes("Ключ шифрования уничтожен")) {
    return { kind: "destroyed", message };
  }
  if (status === 404 || lower.includes("not found")) {
    return { kind: "not_found", message: t("error.notFound") };
  }
  if (status === 410 || lower.includes("expired")) {
    return { kind: "expired", message: t("error.expired") };
  }
  if (status === 403 && (lower.includes("sharing_disabled") || lower.includes("disabled") || lower.includes("отключ"))) {
    return { kind: "blocked", message: t("error.blocked") };
  }
  if (status === 403 && (lower.includes("limit") || lower.includes("download"))) {
    return { kind: "limit_reached", message: t("error.limitReached") };
  }
  if (lower.includes("invalid_password")) {
    return { kind: "invalid_password", message: t("error.invalidPassword") };
  }
  if (status === 401 || lower.includes("password_required")) {
    return { kind: "invalid_password", message: t("error.passwordRequired") };
  }
  if (status === 503 || lower.includes("kms unavailable") || lower.includes("временно недоступен")) {
    return { kind: "unavailable", message: t("error.unavailable") };
  }
  return {
    kind: "generic",
    message: message || t("error.generic"),
  };
}

async function fetchShareInfo(token: string, t: (key: string) => string, password?: string): Promise<ShareInfo> {
  const headers: Record<string, string> = {};
  if (password) headers["X-DataSafe-Share-Password"] = password;
  const res = await fetch(`/api/v1/public/share/${encodeURIComponent(token)}`, { headers });
  const text = await res.text();
  let message = text || res.statusText;
  try {
    const json = JSON.parse(text) as { error?: string };
    if (json.error) message = json.error;
  } catch {
    /* use raw text */
  }
  if (!res.ok) {
    throw classifyShareError(res.status, message, t);
  }
  return JSON.parse(text) as ShareInfo;
}

function ErrorIcon({ kind }: { kind: ShareErrorKind }) {
  const className = "h-6 w-6 text-destructive";
  switch (kind) {
    case "expired":
      return <Clock className={className} />;
    case "limit_reached":
      return <Ban className={className} />;
    case "not_found":
      return <FileIcon className={className} />;
    default:
      return <AlertTriangle className={className} />;
  }
}

export function PublicSharePage() {
  const { t, i18n } = useTranslation("publicShare");
  const { token } = useParams<{ token: string }>();
  const [info, setInfo] = useState<ShareInfo | null>(null);
  const [error, setError] = useState<ShareError | null>(null);
  const [loading, setLoading] = useState(true);
  const [downloading, setDownloading] = useState(false);
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [password, setPassword] = useState("");

  useEffect(() => {
    if (!token) {
      setError({ kind: "not_found", message: t("error.notFound") });
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchShareInfo(token, t)
      .then((data) => {
        if (cancelled) return;
        if (data.password_required && !data.filename) {
          setPasswordRequired(true);
          setInfo(null);
          return;
        }
        setPasswordRequired(false);
        setInfo(data);
      })
      .catch((err: ShareError) => {
        if (!cancelled) setError(err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, t]);

  const unlock = useCallback(async () => {
    if (!token) return;
    setError(null);
    try {
      const data = await fetchShareInfo(token, t, password);
      if (data.password_required && !data.filename) {
        setError({ kind: "generic", message: t("error.invalidPassword") });
        return;
      }
      setPasswordRequired(false);
      setInfo(data);
    } catch (err) {
      setError(err as ShareError);
    }
  }, [password, t, token]);

  const handleDownload = useCallback(async () => {
    if (!token) return;
    setDownloading(true);
    try {
      const headers: Record<string, string> = {};
      if (password) headers["X-DataSafe-Share-Password"] = password;
      const res = await fetch(`/api/v1/public/share/${encodeURIComponent(token)}/download`, { headers });
      if (!res.ok) {
        const text = await res.text();
        let message = text;
        try {
          const json = JSON.parse(text) as { error?: string };
          if (json.error) message = json.error;
        } catch {
          /* raw */
        }
        setError(classifyShareError(res.status, message, t));
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = info?.filename ?? "download";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
    } finally {
      window.setTimeout(() => setDownloading(false), 400);
    }
  }, [info?.filename, password, t, token]);

  const downloadsRemaining =
    info && info.max_downloads > 0
      ? Math.max(0, info.max_downloads - info.download_count)
      : null;
  const showPasswordForm = !loading && passwordRequired && !info;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-3 text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
            {loading ? (
              <Loader2 className="h-6 w-6 animate-spin text-primary" />
            ) : error ? (
              <ErrorIcon kind={error.kind} />
            ) : (
              <Database className="h-6 w-6 text-primary" />
            )}
          </div>
          <CardTitle className="text-2xl">
            {loading
              ? t("brand")
              : showPasswordForm
                ? t("brand")
                : error
                  ? t("title.shared")
                  : info?.filename ?? t("brand")}
          </CardTitle>
          <CardDescription>
            {loading
              ? t("loading")
              : showPasswordForm && error?.kind === "invalid_password"
                ? t("error.invalidPassword")
                : showPasswordForm
                  ? t("hint.password")
                  : error
                    ? error.message
                    : t("ready")}
          </CardDescription>
        </CardHeader>
        {error?.kind === "destroyed" && !loading ? null : (
        <CardContent>
          {loading && (
            <div className="flex justify-center py-4">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          )}

          {!loading && error && error.kind !== "invalid_password" && error.kind !== "destroyed" && (
            <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-center text-sm text-muted-foreground">
              {error.kind === "limit_reached" && <p>{t("hint.limitReached")}</p>}
              {error.kind === "expired" && <p>{t("hint.expired")}</p>}
              {error.kind === "not_found" && <p>{t("hint.notFound")}</p>}
              {error.kind === "generic" && <p>{t("hint.generic")}</p>}
              {error.kind === "blocked" && <p>{t("hint.blocked")}</p>}
              {error.kind === "unavailable" && <p>{t("hint.unavailable")}</p>}
            </div>
          )}

          {!loading && passwordRequired && !info && (
            <div className="space-y-3">
              <p className="text-sm text-muted-foreground">{t("hint.password")}</p>
              <input
                type="password"
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
              <Button className="w-full" onClick={() => void unlock()}>
                {t("download")}
              </Button>
            </div>
          )}

          {!loading && info && (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted/30 p-4 space-y-2 text-sm">
                <div className="flex justify-between gap-4">
                  <span className="text-muted-foreground">{t("fields.size")}</span>
                  <span className="font-medium">{formatBytes(info.size, i18n.language)}</span>
                </div>
                {info.content_type && (
                  <div className="flex justify-between gap-4">
                    <span className="text-muted-foreground">{t("fields.type")}</span>
                    <span className="font-medium truncate max-w-[60%]">{info.content_type}</span>
                  </div>
                )}
                {info.expires_at && (
                  <div className="flex justify-between gap-4">
                    <span className="text-muted-foreground">{t("fields.expires")}</span>
                    <span className="font-medium">{formatDate(info.expires_at, i18n.language)}</span>
                  </div>
                )}
                {downloadsRemaining !== null && (
                  <div className="flex justify-between gap-4">
                    <span className="text-muted-foreground">{t("fields.remaining")}</span>
                    <span className="font-medium">
                      {downloadsRemaining} / {info.max_downloads}
                    </span>
                  </div>
                )}
              </div>
              <Button className="w-full" onClick={handleDownload} disabled={downloading}>
                {downloading ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("downloading")}
                  </>
                ) : (
                  <>
                    <Download className="h-4 w-4" />
                    {t("download")}
                  </>
                )}
              </Button>
            </div>
          )}
        </CardContent>
        )}
      </Card>
    </div>
  );
}
