CREATE TABLE IF NOT EXISTS kms_jobs (
    id         TEXT PRIMARY KEY,
    bucket_key TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT '',
    doc        JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_kms_jobs_bucket ON kms_jobs (bucket_key);
CREATE INDEX IF NOT EXISTS idx_kms_jobs_status ON kms_jobs (status);
