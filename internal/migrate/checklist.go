package migrate

// ChecklistMarkdown returns the MinIO → DataSafeS3 cutover checklist (EN).
func ChecklistMarkdown() string {
	return "# MinIO → DataSafeS3 cutover checklist\n" +
		"\n" +
		"## Before sync\n" +
		"- [ ] DataSafeS3 healthy (GET /healthz)\n" +
		"- [ ] Production metadata: PostgreSQL recommended\n" +
		"- [ ] Target buckets created (same names as source recommended)\n" +
		"- [ ] DataSafe S3 access keys issued (least privilege)\n" +
		"- [ ] Operator host can reach source MinIO and DataSafe endpoints\n" +
		"- [ ] rclone or AWS CLI installed\n" +
		"\n" +
		"## Initial sync\n" +
		"- [ ] Configure remotes (see migrate-from-minio guide)\n" +
		"- [ ] rclone sync (or aws s3 sync) for each bucket\n" +
		"- [ ] Spot-check sample objects (size / content-type)\n" +
		"\n" +
		"## Cutover\n" +
		"- [ ] Freeze or pause writers on MinIO (maintenance window)\n" +
		"- [ ] Final sync with --checksum (or equivalent)\n" +
		"- [ ] Run scripts/migrate/minio-cutover-smoke.ps1 — expect PASS\n" +
		"- [ ] Point applications to DataSafe endpoint / DNS\n" +
		"- [ ] Unfreeze writers against DataSafe only\n" +
		"\n" +
		"## After cutover\n" +
		"- [ ] Re-enable Object Lock / versioning on target if required (not auto-copied as MinIO server config)\n" +
		"- [ ] Remap IAM: MinIO users/policies → DataSafe users, teams, tenants, bucket policies\n" +
		"- [ ] Keep MinIO read-only for rollback window (e.g. 7–14 days)\n" +
		"- [ ] Do not delete source until smoke PASS and apps stable\n" +
		"\n" +
		"## Out of scope (manual)\n" +
		"- MinIO IAM users/groups import\n" +
		"- MinIO console bookmarks / server config\n" +
		"- Claiming 100% MinIO API parity\n"
}
