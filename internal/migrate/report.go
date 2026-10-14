package migrate

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Evidence is the migration report. verdicts is PASS or FAIL per check.
type Evidence struct {
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Duration    string            `json:"duration"`
	Source      string            `json:"source"`
	Target      string            `json:"target"`
	Tool        string            `json:"tool"`
	Buckets     []BucketEvidence  `json:"buckets"`
	Warnings    []string          `json:"warnings,omitempty"`
	Errors      []string          `json:"errors,omitempty"`
	Verdicts    map[string]string `json:"verdicts"`
	Interrupted bool              `json:"interrupted,omitempty"`
}

// BucketEvidence compares one bucket.
type BucketEvidence struct {
	Name             string `json:"name"`
	SourceObjects    int    `json:"source_objects"`
	TargetObjects    int    `json:"target_objects"`
	SourceBytes      int64  `json:"source_bytes"`
	TargetBytes      int64  `json:"target_bytes"`
	SourceMarkers    int    `json:"source_delete_markers"`
	TargetMarkers    int    `json:"target_delete_markers"`
	ETagMismatches   int    `json:"etag_mismatches"`
	LockMismatches   int    `json:"lock_mismatches"`
	SampleChecked    int    `json:"sample_checked"`
	SampleMismatches int    `json:"sample_mismatches"`
}

func (e *Evidence) finish() {
	e.FinishedAt = time.Now().UTC()
	e.Duration = e.FinishedAt.Sub(e.StartedAt).Round(time.Millisecond).String()
	if e.Verdicts == nil {
		e.Verdicts = map[string]string{}
	}
	pass := func(ok bool) string {
		if ok {
			return "PASS"
		}
		return "FAIL"
	}
	objects, bytesOK, markers, etag, lock, sample := true, true, true, true, true, true
	for _, b := range e.Buckets {
		if b.SourceObjects != b.TargetObjects {
			objects = false
		}
		if b.SourceBytes != b.TargetBytes {
			bytesOK = false
		}
		if b.SourceMarkers != b.TargetMarkers {
			markers = false
		}
		if b.ETagMismatches > 0 {
			etag = false
		}
		if b.LockMismatches > 0 {
			lock = false
		}
		if b.SampleMismatches > 0 {
			sample = false
		}
	}
	if e.Interrupted || len(e.Errors) > 0 {
		objects, bytesOK, markers, etag, lock, sample = false, false, false, false, false, false
	}
	e.Verdicts["object_count"] = pass(objects)
	e.Verdicts["bytes"] = pass(bytesOK)
	e.Verdicts["delete_markers"] = pass(markers)
	e.Verdicts["etag"] = pass(etag)
	e.Verdicts["object_lock"] = pass(lock)
	e.Verdicts["content_sample"] = pass(sample)
}

func (e Evidence) OK() bool {
	for _, v := range e.Verdicts {
		if v != "PASS" {
			return false
		}
	}
	return len(e.Verdicts) > 0 && !e.Interrupted
}

func (e Evidence) summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MinIO migration\n\n")
	fmt.Fprintf(&b, "- Source: %s\n- Target: %s\n- Duration: %s\n\n", e.Source, e.Target, e.Duration)
	fmt.Fprintf(&b, "## Verdicts\n\n")
	for _, name := range []string{"object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample"} {
		fmt.Fprintf(&b, "- %s: %s\n", name, e.Verdicts[name])
	}
	fmt.Fprintf(&b, "\n## Buckets\n\n")
	for _, bucket := range e.Buckets {
		fmt.Fprintf(&b, "### %s\n\n", bucket.Name)
		fmt.Fprintf(&b, "- Objects: source %d, target %d\n", bucket.SourceObjects, bucket.TargetObjects)
		fmt.Fprintf(&b, "- Bytes: source %d, target %d\n", bucket.SourceBytes, bucket.TargetBytes)
		fmt.Fprintf(&b, "- Delete markers: source %d, target %d\n", bucket.SourceMarkers, bucket.TargetMarkers)
		fmt.Fprintf(&b, "- ETag mismatches: %d\n", bucket.ETagMismatches)
		fmt.Fprintf(&b, "- Object Lock mismatches: %d\n", bucket.LockMismatches)
		fmt.Fprintf(&b, "- Content samples: %d checked, %d mismatches\n\n", bucket.SampleChecked, bucket.SampleMismatches)
	}
	if len(e.Warnings) > 0 {
		fmt.Fprintf(&b, "## Warnings\n\n")
		for _, w := range e.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
		b.WriteString("\n")
	}
	if len(e.Errors) > 0 {
		fmt.Fprintf(&b, "## Errors\n\n")
		for _, w := range e.Errors {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	return b.String()
}

func writeReport(dir string, ev Evidence) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	md := ev.summary()
	raw, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(md), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence.json"), raw, 0o644); err != nil {
		return err
	}
	zf, err := os.Create(filepath.Join(dir, "evidence.zip"))
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	for name, body := range map[string][]byte{"summary.md": []byte(md), "evidence.json": raw} {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return zw.Close()
}
