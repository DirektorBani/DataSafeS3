import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const verdictNames = ["object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample"] as const;

type EvidenceFile = {
  verdicts?: Record<string, string>;
  warnings?: string[];
  errors?: string[];
  interrupted?: boolean;
};

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

export function buildMigrateCommand(input: {
  sourceUrl: string;
  targetUrl: string;
  buckets: string;
  sse: string;
  kmsKey: string;
  concurrency: string;
  partSize: string;
  verifySample: string;
  stateDir: string;
  reportDir: string;
}): string {
  const lines = [
    "storage-cli migrate from-minio \\",
    `  --source ${shellQuote(input.sourceUrl.trim() || "http://minio.example:9000")} \\`,
    `  --source-key "$SOURCE_ACCESS_KEY" \\`,
    `  --source-secret "$SOURCE_SECRET_KEY" \\`,
    `  --target ${shellQuote(input.targetUrl.trim() || "http://127.0.0.1:9000")} \\`,
    `  --target-key "$TARGET_ACCESS_KEY" \\`,
    `  --target-secret "$TARGET_SECRET_KEY" \\`,
  ];
  const buckets = input.buckets
    .split(",")
    .map((b) => b.trim())
    .filter(Boolean)
    .join(",");
  if (buckets) {
    lines.push(`  --buckets ${shellQuote(buckets)} \\`);
  }
  if (input.sse === "AES256" || input.sse === "aws:kms") {
    lines.push(`  --sse ${input.sse} \\`);
  }
  if (input.sse === "aws:kms" && input.kmsKey.trim()) {
    lines.push(`  --kms-key ${shellQuote(input.kmsKey.trim())} \\`);
  }
  if (input.concurrency.trim()) {
    lines.push(`  --concurrency ${shellQuote(input.concurrency.trim())} \\`);
  }
  if (input.partSize.trim()) {
    lines.push(`  --part-size ${shellQuote(input.partSize.trim())} \\`);
  }
  if (input.verifySample.trim()) {
    lines.push(`  --verify-sample ${shellQuote(input.verifySample.trim())} \\`);
  }
  lines.push(`  --state ${shellQuote(input.stateDir.trim() || "./.minio-migrate-state")} \\`);
  lines.push(`  --report ${shellQuote(input.reportDir.trim() || "./minio-migrate-report")}`);
  return lines.join("\n");
}

export function MigrateSettingsPage() {
  const { t } = useTranslation("migrate");
  const [sourceUrl, setSourceUrl] = useState("http://minio.example:9000");
  const [targetUrl, setTargetUrl] = useState("http://127.0.0.1:9000");
  const [buckets, setBuckets] = useState("");
  const [sse, setSse] = useState("");
  const [kmsKey, setKmsKey] = useState("");
  const [concurrency, setConcurrency] = useState("4");
  const [partSize, setPartSize] = useState("8MB");
  const [verifySample, setVerifySample] = useState("10");
  const [stateDir, setStateDir] = useState("./.minio-migrate-state");
  const [reportDir, setReportDir] = useState("./minio-migrate-report");
  const [copied, setCopied] = useState(false);
  const [evidence, setEvidence] = useState<EvidenceFile | null>(null);
  const [evidenceError, setEvidenceError] = useState("");

  const command = useMemo(
    () =>
      buildMigrateCommand({
        sourceUrl,
        targetUrl,
        buckets,
        sse,
        kmsKey,
        concurrency,
        partSize,
        verifySample,
        stateDir,
        reportDir,
      }),
    [sourceUrl, targetUrl, buckets, sse, kmsKey, concurrency, partSize, verifySample, stateDir, reportDir],
  );

  return (
    <div className="space-y-4" data-testid="migrate-settings">
      <div>
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-muted-foreground mt-1 max-w-3xl">{t("lead")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("commandTitle")}</CardTitle>
          <CardDescription>{t("commandHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="migrate-source">{t("sourceUrl")}</Label>
              <Input id="migrate-source" data-testid="migrate-source" value={sourceUrl} onChange={(e) => setSourceUrl(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-target">{t("targetUrl")}</Label>
              <Input id="migrate-target" data-testid="migrate-target" value={targetUrl} onChange={(e) => setTargetUrl(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-buckets">{t("buckets")}</Label>
              <Input id="migrate-buckets" data-testid="migrate-buckets" value={buckets} onChange={(e) => setBuckets(e.target.value)} />
              <p className="text-xs text-muted-foreground">{t("bucketsHint")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-sse">{t("sse")}</Label>
              <select
                id="migrate-sse"
                data-testid="migrate-sse"
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm"
                value={sse}
                onChange={(e) => setSse(e.target.value)}
              >
                <option value="">{t("sseNone")}</option>
                <option value="AES256">AES256</option>
                <option value="aws:kms">aws:kms</option>
              </select>
            </div>
            {sse === "aws:kms" && (
              <div className="space-y-2">
                <Label htmlFor="migrate-kms-key">{t("kmsKey")}</Label>
                <Input id="migrate-kms-key" data-testid="migrate-kms-key" value={kmsKey} onChange={(e) => setKmsKey(e.target.value)} />
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="migrate-concurrency">{t("concurrency")}</Label>
              <Input id="migrate-concurrency" value={concurrency} onChange={(e) => setConcurrency(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-part-size">{t("partSize")}</Label>
              <Input id="migrate-part-size" value={partSize} onChange={(e) => setPartSize(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-verify">{t("verifySample")}</Label>
              <Input id="migrate-verify" value={verifySample} onChange={(e) => setVerifySample(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-state">{t("stateDir")}</Label>
              <Input id="migrate-state" value={stateDir} onChange={(e) => setStateDir(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="migrate-report">{t("reportDir")}</Label>
              <Input id="migrate-report" value={reportDir} onChange={(e) => setReportDir(e.target.value)} />
            </div>
          </div>
          <pre data-testid="migrate-command" className="overflow-x-auto rounded-md bg-muted p-3 text-xs">
            {command}
          </pre>
          <Button
            type="button"
            variant="outline"
            data-testid="migrate-copy"
            onClick={() => {
              void navigator.clipboard.writeText(command).then(() => {
                setCopied(true);
              });
            }}
          >
            {copied ? t("copied") : t("copy")}
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("evidenceTitle")}</CardTitle>
          <CardDescription>{t("evidenceHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <Label htmlFor="migrate-evidence-file">{t("evidenceFile")}</Label>
          <Input
            id="migrate-evidence-file"
            data-testid="migrate-evidence-file"
            type="file"
            accept="application/json,.json"
            onChange={(e) => {
              const file = e.target.files?.[0];
              setEvidence(null);
              setEvidenceError("");
              if (!file) return;
              void file.text().then((raw) => {
                try {
                  const parsed = JSON.parse(raw) as EvidenceFile;
                  if (!parsed || typeof parsed !== "object" || !parsed.verdicts) {
                    setEvidenceError(t("evidenceBad"));
                    return;
                  }
                  setEvidence(parsed);
                } catch {
                  setEvidenceError(t("evidenceBad"));
                }
              });
            }}
          />
          {!evidence && !evidenceError && <p className="text-sm text-muted-foreground">{t("evidenceEmpty")}</p>}
          {evidenceError && (
            <p className="text-sm text-destructive" data-testid="migrate-evidence-error">
              {evidenceError}
            </p>
          )}
          {evidence && (
            <div className="space-y-2" data-testid="migrate-evidence">
              <h2 className="text-sm font-medium">{t("verdicts")}</h2>
              <ul className="space-y-1 text-sm">
                {verdictNames.map((name) => (
                  <li key={name} data-testid={`migrate-verdict-${name}`}>
                    {name}: {evidence.verdicts?.[name] ?? "—"}
                  </li>
                ))}
              </ul>
              {evidence.interrupted && <p data-testid="migrate-interrupted">{t("interrupted")}</p>}
              {(evidence.warnings?.length ?? 0) > 0 && (
                <div>
                  <h2 className="text-sm font-medium">{t("warnings")}</h2>
                  <ul className="list-disc pl-5 text-sm">
                    {evidence.warnings?.map((w) => (
                      <li key={w}>{w}</li>
                    ))}
                  </ul>
                </div>
              )}
              {(evidence.errors?.length ?? 0) > 0 && (
                <div>
                  <h2 className="text-sm font-medium">{t("errors")}</h2>
                  <ul className="list-disc pl-5 text-sm">
                    {evidence.errors?.map((w) => (
                      <li key={w}>{w}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
