package inventory

// InventoryJob describes a planned capacity / listing export (stub for A4).
type InventoryJob struct {
	ID         string
	Bucket     string
	Prefix     string
	Format     string // csv|parquet
	Schedule   string // cron or manual
	DestBucket string
	DestKey    string
}
