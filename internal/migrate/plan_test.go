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

func TestOrderVersionsKeepsMarkerBetweenVersions(t *testing.T) {
	in := []ObjectVersion{
		{Bucket: "b", Key: "k", VersionID: "new", Seq: 1},
		{Bucket: "b", Key: "k", VersionID: "mark", Seq: 2, DeleteMarker: true},
		{Bucket: "b", Key: "k", VersionID: "old", Seq: 3},
	}
	out := OrderVersions(in)
	if out[0].VersionID != "old" || !out[1].DeleteMarker || out[2].VersionID != "new" {
		t.Fatalf("marker not interleaved: %+v", out)
	}
}

func TestLifecycleUnsupported(t *testing.T) {
	plain := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>keep</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`
	if err := LifecycleUnsupported(plain); err != nil {
		t.Fatal(err)
	}
	if err := LifecycleUnsupported(`<LifecycleConfiguration><Rule><ID>bad</ID><Status>Enabled</Status><NoncurrentVersionExpiration><NoncurrentDays>1</NoncurrentDays></NoncurrentVersionExpiration></Rule></LifecycleConfiguration>`); err == nil {
		t.Fatal("noncurrent rule should be rejected")
	}
	if err := LifecycleUnsupported(`<LifecycleConfiguration><Rule><Status>Enabled</Status><Transition><Days>1</Days></Transition><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`); err == nil {
		t.Fatal("transition should be rejected")
	}
}

func TestSampleIndexesAtLeastOne(t *testing.T) {
	got := SampleIndexes(5, 10, false)
	if len(got) != 1 {
		t.Fatalf("10%% of 5 objects: %v", got)
	}
	if _, ok := got[0]; !ok {
		t.Fatalf("first object missing: %v", got)
	}
	if len(SampleIndexes(3, 0, true)) != 3 {
		t.Fatal("verify-full should include every object")
	}
}

func TestParseFromMinIOSecretsFromEnv(t *testing.T) {
	t.Setenv("SOURCE_ACCESS_KEY", "src-key")
	t.Setenv("SOURCE_SECRET_KEY", "src-secret")
	t.Setenv("TARGET_ACCESS_KEY", "dst-key")
	t.Setenv("TARGET_SECRET_KEY", "dst-secret")
	opt, err := ParseFromMinIO([]string{"--source", "http://minio", "--target", "http://datasafe"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.Source.AccessKey != "src-key" || opt.Source.SecretKey != "src-secret" || opt.Targets[0].SecretKey != "dst-secret" {
		t.Fatalf("%+v", opt)
	}
	opt, err = ParseFromMinIO([]string{"--source", "http://minio", "--source-key", "flag", "--source-secret", "flag-secret", "--target", "http://datasafe", "--target-key", "flag", "--target-secret", "flag-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.Source.SecretKey != "flag-secret" {
		t.Fatalf("flag should win: %s", opt.Source.SecretKey)
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
	for _, name := range []string{"object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample", "encryption"} {
		if ev.Verdicts[name] != "PASS" {
			t.Fatalf("%s %s", name, ev.Verdicts[name])
		}
	}
	ev.Interrupted = true
	ev.finish()
	if ev.OK() || ev.Verdicts["object_count"] != "FAIL" || ev.Verdicts["encryption"] != "FAIL" {
		t.Fatalf("interrupted should fail: %+v", ev.Verdicts)
	}
	ev = Evidence{Buckets: []BucketEvidence{{
		SourceObjects: 1, TargetObjects: 1, SourceBytes: 4, TargetBytes: 4,
		EncryptionMismatches: 1,
	}}}
	ev.finish()
	if ev.OK() || ev.Verdicts["encryption"] != "FAIL" || ev.Verdicts["etag"] != "PASS" {
		t.Fatalf("encryption loss should fail the run: %+v", ev.Verdicts)
	}
}
