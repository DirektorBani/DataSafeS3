export type PolicyEffect = "Allow" | "Deny";

export type PolicyStatement = {
  id: string;
  Effect: PolicyEffect;
  Principal: string;
  Action: string[];
  Resource: string[];
};

export type PolicyDocument = {
  Version: string;
  Statement: PolicyStatement[];
};

export const S3_ACTIONS = [
  { id: "s3:ListBucket", label: "List bucket" },
  { id: "s3:GetObject", label: "Get object" },
  { id: "s3:PutObject", label: "Put object" },
  { id: "s3:DeleteObject", label: "Delete object" },
  { id: "s3:*", label: "All S3 actions" },
] as const;

export function bucketArn(bucket: string): string {
  return `arn:aws:s3:::${bucket}`;
}

export function objectArn(bucket: string, prefix = "*"): string {
  const suffix = prefix === "*" || prefix === "" ? "*" : prefix.replace(/\/$/, "") + "/*";
  return `arn:aws:s3:::${bucket}/${suffix}`;
}

export function emptyPolicy(): PolicyDocument {
  return { Version: "2012-10-17", Statement: [] };
}

export function newStatement(bucket: string): PolicyStatement {
  return {
    id: crypto.randomUUID(),
    Effect: "Allow",
    Principal: "*",
    Action: ["s3:GetObject"],
    Resource: [objectArn(bucket)],
  };
}

export function parsePolicy(json: string): PolicyDocument {
  if (!json.trim()) return emptyPolicy();
  const parsed = JSON.parse(json) as PolicyDocument;
  if (!parsed.Version) parsed.Version = "2012-10-17";
  if (!Array.isArray(parsed.Statement)) parsed.Statement = [];
  parsed.Statement = parsed.Statement.map((st, i) => ({
    id: (st as PolicyStatement).id || `stmt-${i}`,
    Effect: st.Effect === "Deny" ? "Deny" : "Allow",
    Principal: typeof st.Principal === "string" ? st.Principal : "*",
    Action: Array.isArray(st.Action) ? st.Action : st.Action ? [String(st.Action)] : [],
    Resource: Array.isArray(st.Resource) ? st.Resource : st.Resource ? [String(st.Resource)] : [],
  }));
  return parsed;
}

export function serializePolicy(doc: PolicyDocument): string {
  const out = {
    Version: doc.Version || "2012-10-17",
    Statement: doc.Statement.map(({ Effect, Principal, Action, Resource }) => ({
      Effect,
      Principal,
      Action,
      Resource,
    })),
  };
  return JSON.stringify(out, null, 2);
}
