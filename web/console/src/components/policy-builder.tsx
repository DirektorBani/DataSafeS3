import { Plus, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  S3_ACTIONS,
  bucketArn,
  newStatement,
  objectArn,
  type PolicyDocument,
  type PolicyStatement,
} from "@/lib/policy";

type PolicyBuilderProps = {
  bucket: string;
  document: PolicyDocument;
  onChange: (doc: PolicyDocument) => void;
};

const S3_ACTION_KEYS: Record<string, string> = {
  "s3:ListBucket": "s3Actions.listBucket",
  "s3:GetObject": "s3Actions.getObject",
  "s3:PutObject": "s3Actions.putObject",
  "s3:DeleteObject": "s3Actions.deleteObject",
  "s3:*": "s3Actions.allS3",
};

function StatementCard({
  bucket,
  statement,
  onChange,
  onRemove,
}: {
  bucket: string;
  statement: PolicyStatement;
  onChange: (st: PolicyStatement) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation("policyBuilder");

  const toggleAction = (action: string) => {
    const has = statement.Action.includes(action);
    onChange({
      ...statement,
      Action: has ? statement.Action.filter((a) => a !== action) : [...statement.Action, action],
    });
  };

  const setResourcePreset = (preset: "bucket" | "objects" | "custom") => {
    if (preset === "bucket") onChange({ ...statement, Resource: [bucketArn(bucket)] });
    else if (preset === "objects") onChange({ ...statement, Resource: [objectArn(bucket)] });
  };

  const applyPreset = (preset: "read" | "write" | "delete" | "list" | "admin") => {
    const presets: Record<string, string[]> = {
      read: ["s3:GetObject"],
      write: ["s3:PutObject"],
      delete: ["s3:DeleteObject"],
      list: ["s3:ListBucket"],
      admin: ["s3:*"],
    };
    onChange({ ...statement, Action: presets[preset] ?? statement.Action });
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="text-sm font-medium">{t("statement")}</CardTitle>
        <Button variant="ghost" size="sm" className="text-destructive" onClick={onRemove}>
          <Trash2 className="h-4 w-4" />
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>{t("effect")}</Label>
            <Select
              value={statement.Effect}
              onValueChange={(v) => onChange({ ...statement, Effect: v as "Allow" | "Deny" })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="Allow">{t("effectAllow")}</SelectItem>
                <SelectItem value="Deny">{t("effectDeny")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t("principal")}</Label>
            <Input
              value={statement.Principal}
              onChange={(e) => onChange({ ...statement, Principal: e.target.value })}
              placeholder="*"
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t("quickPresets")}</Label>
          <div className="flex flex-wrap gap-2">
            {(["read", "write", "delete", "list", "admin"] as const).map((p) => (
              <Button key={p} type="button" variant="outline" size="sm" onClick={() => applyPreset(p)}>
                {t(`presets.${p}`)}
              </Button>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t("actions")}</Label>
          <div className="grid gap-2 sm:grid-cols-2">
            {S3_ACTIONS.map((a) => (
              <label key={a.id} className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  className="rounded border-input"
                  checked={statement.Action.includes(a.id)}
                  onChange={() => toggleAction(a.id)}
                />
                <span>{t(S3_ACTION_KEYS[a.id] ?? a.id)}</span>
                <code className="text-xs text-muted-foreground">{a.id}</code>
              </label>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t("resources")}</Label>
          <div className="flex flex-wrap gap-2 mb-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setResourcePreset("bucket")}>
              {t("bucketArn")}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setResourcePreset("objects")}>
              {t("allObjects")}
            </Button>
          </div>
          {statement.Resource.map((res, idx) => (
            <Input
              key={idx}
              value={res}
              onChange={(e) => {
                const next = [...statement.Resource];
                next[idx] = e.target.value;
                onChange({ ...statement, Resource: next });
              }}
              className="font-mono text-xs"
            />
          ))}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onChange({ ...statement, Resource: [...statement.Resource, objectArn(bucket)] })}
          >
            {t("addResource")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function PolicyBuilder({ bucket, document, onChange }: PolicyBuilderProps) {
  const { t } = useTranslation("policyBuilder");

  const addStatement = () => {
    onChange({ ...document, Statement: [...document.Statement, newStatement(bucket)] });
  };

  const updateStatement = (idx: number, st: PolicyStatement) => {
    const next = [...document.Statement];
    next[idx] = st;
    onChange({ ...document, Statement: next });
  };

  const removeStatement = (idx: number) => {
    onChange({ ...document, Statement: document.Statement.filter((_, i) => i !== idx) });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {t("intro", { bucket })}
        </p>
        <Button type="button" size="sm" variant="outline" onClick={addStatement}>
          <Plus className="h-4 w-4" />
          {t("addStatement")}
        </Button>
      </div>
      {document.Statement.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            {t("empty")}
          </CardContent>
        </Card>
      ) : (
        document.Statement.map((st, idx) => (
          <StatementCard
            key={st.id}
            bucket={bucket}
            statement={st}
            onChange={(updated) => updateStatement(idx, updated)}
            onRemove={() => removeStatement(idx)}
          />
        ))
      )}
    </div>
  );
}
