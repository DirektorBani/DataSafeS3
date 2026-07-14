package inventory

import "testing"

func TestInventoryJobFields(t *testing.T) {
	j := InventoryJob{
		ID:         "job-1",
		Bucket:     "reports",
		Prefix:     "2026/",
		Format:     "csv",
		Schedule:   "manual",
		DestBucket: "inventory",
		DestKey:    "reports/2026.csv",
	}
	if j.ID == "" || j.Bucket == "" || j.DestBucket == "" {
		t.Fatal("required fields empty")
	}
	if j.Format != "csv" && j.Format != "parquet" {
		t.Fatalf("unexpected format %q", j.Format)
	}
}

func TestInventoryJobZeroValue(t *testing.T) {
	var j InventoryJob
	if j.Schedule != "" {
		t.Fatal("zero value should have empty schedule")
	}
}
