package migrate

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestOrderVersionsOldestFirst(t *testing.T) {
	newer := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []ObjectVersion{
		{Bucket: "b", Key: "k", VersionID: "2", LastModified: newer},
		{Bucket: "b", Key: "k", VersionID: "1", LastModified: older},
		{Bucket: "a", Key: "z", VersionID: "9", LastModified: newer},
	}
	out := OrderVersions(in)
	if out[0].Bucket != "a" || out[1].VersionID != "1" || out[2].VersionID != "2" {
		t.Fatalf("order %+v", out)
	}
}

func TestPortPolicyDropsUnsupportedCondition(t *testing.T) {
	raw := `{
	  "Version": "2012-10-17",
	  "Statement": [
	    {"Sid":"Keep","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::b/*","Condition":{"IpAddress":{"aws:SourceIp":"10.0.0.0/8"}}},
	    {"Sid":"DropMe","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::b/*","Condition":{"StringEquals":{"s3:prefix":"x"}}}
	  ]
	}`
	ported, warns := PortPolicy(raw)
	if strings.Contains(ported, "DropMe") || strings.Contains(ported, "StringEquals") {
		t.Fatalf("unsupported statement kept: %s", ported)
	}
	if !strings.Contains(ported, "Keep") {
		t.Fatalf("supported statement dropped: %s", ported)
	}
	if len(warns) == 0 || !strings.Contains(warns[0], "DropMe") {
		t.Fatalf("warnings %v", warns)
	}
}

func TestLifecycleWarning(t *testing.T) {
	if LifecycleWarning(`<Expiration><Days>1</Days></Expiration>`) != "" {
		t.Fatal("plain expiration should not warn")
	}
	if !strings.Contains(LifecycleWarning(`<Transition><Days>1</Days></Transition>`), "not copied") {
		t.Fatal("transition should warn")
	}
}

func TestS3MultipartETag(t *testing.T) {
	body := []byte("abcdefghij")
	partSize := int64(4)
	var hashes [][]byte
	for off := 0; off < len(body); off += int(partSize) {
		end := off + int(partSize)
		if end > len(body) {
			end = len(body)
		}
		sum := md5.Sum(body[off:end])
		hashes = append(hashes, append([]byte(nil), sum[:]...))
	}
	h := md5.New()
	for _, p := range hashes {
		h.Write(p)
	}
	want := hex.EncodeToString(h.Sum(nil)) + "-3"
	if got := S3MultipartETag(body, partSize); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	single := md5.Sum(body)
	if got := S3MultipartETag(body, 100); got != hex.EncodeToString(single[:]) {
		t.Fatalf("single-part etag %s", got)
	}
	if !ETagsAgree(want, "different", body, partSize) {
		t.Fatal("multipart source etag should match a recomputed body")
	}
	if ETagsAgree("abcd", "ffff", body, partSize) {
		t.Fatal("plain etag mismatch should fail")
	}
}

func TestParseFromMinIORequiresEndpoints(t *testing.T) {
	if _, err := ParseFromMinIO(nil); err == nil {
		t.Fatal("expected missing --source")
	}
	_, err := ParseFromMinIO([]string{
		"--source", "http://minio",
		"--source-key", "k",
		"--source-secret", "s",
		"--target", "http://ds",
		"--target-key", "k",
		"--target-secret", "s",
		"--sse", "aws:kms",
	})
	if err == nil || !strings.Contains(err.Error(), "--kms-key") {
		t.Fatalf("got %v", err)
	}
}

func TestEvidenceFinishVerdicts(t *testing.T) {
	ev := Evidence{Buckets: []BucketEvidence{{
		SourceObjects: 1, TargetObjects: 1,
		SourceBytes: 4, TargetBytes: 4,
		SourceMarkers: 1, TargetMarkers: 1,
	}}}
	ev.finish()
	for _, name := range []string{"object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample"} {
		if ev.Verdicts[name] != "PASS" {
			t.Fatalf("%s %s", name, ev.Verdicts[name])
		}
	}
	ev.Interrupted = true
	ev.finish()
	if ev.OK() || ev.Verdicts["object_count"] != "FAIL" {
		t.Fatalf("interrupted should fail: %+v", ev.Verdicts)
	}
}
