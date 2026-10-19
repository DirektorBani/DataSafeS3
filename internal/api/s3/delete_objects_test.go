package s3_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestDeleteObjectsRemovesKeysQuietAndMissing(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "multi")
	putObject(t, ts, creds, "/multi/a.txt", "aaa")
	putObject(t, ts, creds, "/multi/b.txt", "bbb")

	var outbox atomic.Int32
	srv.Svc().ObjectOutbox = func(_ context.Context, _ metadata.BucketRecord, obj metadata.ObjectRecord, kind string) []events.OutboxRecord {
		if kind == "delete" {
			outbox.Add(1)
			if obj.Key == "" {
				t.Errorf("outbox delete missing key")
			}
		}
		return nil
	}

	body := `<Delete><Object><Key>a.txt</Key></Object><Object><Key>b.txt</Key></Object><Object><Key>missing.txt</Key></Object></Delete>`
	resp, raw := postDelete(t, ts, creds, "/multi?delete", body, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d %s", resp.StatusCode, raw)
	}
	var doc deleteResultDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Deleted) != 3 || len(doc.Errors) != 0 {
		t.Fatalf("deleted=%d errors=%d body=%s", len(doc.Deleted), len(doc.Errors), raw)
	}
	if outbox.Load() != 2 {
		t.Fatalf("outbox deletes %d, want 2 (missing key is a no-op)", outbox.Load())
	}
	if code := getStatus(t, ts, creds, "/multi/a.txt"); code != http.StatusNotFound {
		t.Fatalf("a.txt status %d", code)
	}

	quiet := `<Delete><Quiet>true</Quiet><Object><Key>gone.txt</Key></Object></Delete>`
	resp, raw = postDelete(t, ts, creds, "/multi?delete", quiet, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("quiet status %d %s", resp.StatusCode, raw)
	}
	if strings.Contains(string(raw), "<Deleted>") || strings.Contains(string(raw), "<Key>") {
		t.Fatalf("quiet success leaked into the body: %s", raw)
	}
}

func TestDeleteObjectsChecksumsAndLimit(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "sums")
	putObject(t, ts, creds, "/sums/k", "v")

	body := `<Delete><Object><Key>k</Key></Object></Delete>`
	resp, raw := postDelete(t, ts, creds, "/sums?delete", body, false)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "InvalidRequest") {
		t.Fatalf("missing checksum %d %s", resp.StatusCode, raw)
	}

	req := signedDelete(t, ts.URL+"/sums?delete", creds, body)
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString([]byte("not-the-md5!!!!")))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "BadDigest") {
		t.Fatalf("bad md5 %d %s", resp.StatusCode, raw)
	}

	sum := md5.Sum([]byte(body))
	req = signedDelete(t, ts.URL+"/sums?delete", creds, body)
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("good md5 %d %s", resp.StatusCode, raw)
	}

	putObject(t, ts, creds, "/sums/c", "v")
	body = `<Delete><Object><Key>c</Key></Object></Delete>`
	h := crc32.NewIEEE()
	_, _ = h.Write([]byte(body))
	req = signedDelete(t, ts.URL+"/sums?delete", creds, body)
	req.Header.Set("x-amz-checksum-crc32", base64.StdEncoding.EncodeToString(h.Sum(nil)))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("crc32 %d %s", resp.StatusCode, raw)
	}

	var b strings.Builder
	b.WriteString("<Delete>")
	for i := 0; i < 1001; i++ {
		b.WriteString("<Object><Key>k</Key></Object>")
	}
	b.WriteString("</Delete>")
	resp, raw = postDelete(t, ts, creds, "/sums?delete", b.String(), true)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "1000") {
		t.Fatalf("limit %d %s", resp.StatusCode, raw)
	}
}

func TestDeleteObjectsVersioningLockBypassAndPolicy(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "vers")
	put(t, ts.URL, creds, "/vers?versioning", `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
	putObject(t, ts, creds, "/vers/doc.txt", "one")
	body := `<Delete><Object><Key>doc.txt</Key></Object></Delete>`
	resp, raw := postDelete(t, ts, creds, "/vers?delete", body, true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "<DeleteMarker>true</DeleteMarker>") || !strings.Contains(string(raw), "DeleteMarkerVersionId") {
		t.Fatalf("marker %d %s", resp.StatusCode, raw)
	}
	var doc deleteResultDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Deleted) != 1 || doc.Deleted[0].DeleteMarkerVersionID == "" {
		t.Fatalf("marker id: %s", raw)
	}
	marker := doc.Deleted[0].DeleteMarkerVersionID
	body = `<Delete><Object><Key>doc.txt</Key><VersionId>` + marker + `</VersionId></Object></Delete>`
	resp, raw = postDelete(t, ts, creds, "/vers?delete", body, true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "<DeleteMarker>true</DeleteMarker>") {
		t.Fatalf("remove marker %d %s", resp.StatusCode, raw)
	}

	setupBucket(t, ts, creds, "lock")
	brecLock, err := srv.Meta().GetBucket("lock")
	if err != nil {
		t.Fatal(err)
	}
	brecLock.ObjectLock = true
	brecLock.Versioning = true
	if err := srv.Meta().UpdateBucket(brecLock); err != nil {
		t.Fatal(err)
	}
	putObject(t, ts, creds, "/lock/file.txt", "locked")
	until := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	put(t, ts.URL, creds, "/lock/file.txt?retention", `<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>`+until+`</RetainUntilDate></Retention>`)
	body = `<Delete><Object><Key>file.txt</Key></Object></Delete>`
	resp, raw = postDelete(t, ts, creds, "/lock?delete", body, true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "AccessDenied") {
		t.Fatalf("governance block %d %s", resp.StatusCode, raw)
	}
	if code := getStatus(t, ts, creds, "/lock/file.txt"); code != http.StatusOK {
		t.Fatalf("object removed despite governance, status %d", code)
	}
	blocked, err := srv.Meta().ListActivity(metadata.ActivityFilter{Action: metadata.ActionObjectDeleteBlocked, Period: "all", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Total < 1 {
		t.Fatalf("expected object_delete_blocked activity, got %+v", blocked.Events)
	}

	req := signedDelete(t, ts.URL+"/lock?delete", creds, body)
	req.Header.Set("x-amz-bypass-governance-retention", "true")
	sum := md5.Sum([]byte(body))
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(raw), "<Error>") {
		t.Fatalf("bypass %d %s", resp.StatusCode, raw)
	}
	if code, rawGet := getStatusBody(t, ts, creds, "/lock/file.txt"); code != http.StatusNotFound {
		t.Fatalf("bypass left the object, status %d %s", code, rawGet)
	}

	putObject(t, ts, creds, "/lock/held.txt", "held")
	put(t, ts.URL, creds, "/lock/held.txt?legal-hold", `<LegalHold><Status>ON</Status></LegalHold>`)
	body = `<Delete><Object><Key>held.txt</Key></Object></Delete>`
	req = signedDelete(t, ts.URL+"/lock?delete", creds, body)
	req.Header.Set("x-amz-bypass-governance-retention", "true")
	sum = md5.Sum([]byte(body))
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if !strings.Contains(string(raw), "legal hold") {
		t.Fatalf("legal hold bypassed: %s", raw)
	}

	putObject(t, ts, creds, "/lock/comp.txt", "comp")
	until = time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	put(t, ts.URL, creds, "/lock/comp.txt?retention", `<Retention><Mode>COMPLIANCE</Mode><RetainUntilDate>`+until+`</RetainUntilDate></Retention>`)
	body = `<Delete><Object><Key>comp.txt</Key></Object></Delete>`
	req = signedDelete(t, ts.URL+"/lock?delete", creds, body)
	req.Header.Set("x-amz-bypass-governance-retention", "true")
	sum = md5.Sum([]byte(body))
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	signRequest(t, req, creds, "us-east-1")
	resp, raw = doRaw(t, req)
	if !strings.Contains(string(raw), "AccessDenied") {
		t.Fatalf("compliance bypassed: %s", raw)
	}

	setupBucket(t, ts, creds, "pol")
	putObject(t, ts, creds, "/pol/ok/a", "yes")
	putObject(t, ts, creds, "/pol/no/b", "no")
	if err := srv.Meta().PutAccessKey(metadata.AccessKeyRecord{
		AccessKey: "limited", SecretKey: "limitedsecret", Label: "limited", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	brec, err := srv.Meta().GetBucket("pol")
	if err != nil {
		t.Fatal(err)
	}
	brec.Policy = `{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::datasafe:access-key/limited"},"Action":["s3:DeleteObject","s3:GetObject","s3:PutObject"],"Resource":["arn:aws:s3:::pol/ok/*"]},
			{"Effect":"Deny","Principal":{"AWS":"arn:aws:iam::datasafe:access-key/limited"},"Action":["s3:DeleteObject"],"Resource":["arn:aws:s3:::pol/no/*"]}
		]
	}`
	if err := srv.Meta().UpdateBucket(brec); err != nil {
		t.Fatal(err)
	}
	limited := auth.Credentials{AccessKey: "limited", SecretKey: "limitedsecret"}
	body = `<Delete><Object><Key>ok/a</Key></Object><Object><Key>no/b</Key></Object></Delete>`
	resp, raw = postDelete(t, ts, limited, "/pol?delete", body, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policy status %d %s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "<Key>ok/a</Key>") || !strings.Contains(string(raw), "AccessDenied") {
		t.Fatalf("per-key policy: %s", raw)
	}
	if code := getStatus(t, ts, creds, "/pol/no/b"); code != http.StatusOK {
		t.Fatalf("denied key was deleted, status %d", code)
	}
	if code := getStatus(t, ts, creds, "/pol/ok/a"); code != http.StatusNotFound {
		t.Fatalf("allowed key remains, status %d", code)
	}
}

func TestAWSSDKDeleteObjects(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(creds.AccessKey, creds.SecretKey, "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(ts.URL)
		o.UsePathStyle = true
	})
	bucket := "sdk-del"
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"one", "dir/two"} {
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(key),
		}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &s3types.Delete{
			Objects: []s3types.ObjectIdentifier{
				{Key: aws.String("one")},
				{Key: aws.String("dir/two")},
				{Key: aws.String("absent")},
			},
		},
	})
	if err != nil {
		t.Fatalf("DeleteObjects: %v", err)
	}
	if len(out.Deleted) != 3 || len(out.Errors) != 0 {
		t.Fatalf("deleted %d errors %d", len(out.Deleted), len(out.Errors))
	}
	if _, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String("one")}); err == nil {
		t.Fatal("one still exists")
	}
}

type deleteResultDoc struct {
	Deleted []struct {
		Key                   string `xml:"Key"`
		DeleteMarker          bool   `xml:"DeleteMarker"`
		DeleteMarkerVersionID string `xml:"DeleteMarkerVersionId"`
	} `xml:"Deleted"`
	Errors []struct {
		Key  string `xml:"Key"`
		Code string `xml:"Code"`
	} `xml:"Error"`
}

func putObject(t *testing.T, ts *httptest.Server, creds auth.Credentials, path, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("put %s %d %s", path, resp.StatusCode, raw)
	}
}

func postDelete(t *testing.T, ts *httptest.Server, creds auth.Credentials, path, body string, withMD5 bool) (*http.Response, []byte) {
	t.Helper()
	req := signedDelete(t, ts.URL+path, creds, body)
	if withMD5 {
		sum := md5.Sum([]byte(body))
		req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	}
	signRequest(t, req, creds, "us-east-1")
	return doRaw(t, req)
}

func signedDelete(t *testing.T, url string, creds auth.Credentials, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/xml")
	_ = creds
	return req
}

func doRaw(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, raw
}

func getStatus(t *testing.T, ts *httptest.Server, creds auth.Credentials, path string) int {
	t.Helper()
	code, _ := getStatusBody(t, ts, creds, path)
	return code
}

func getStatusBody(t *testing.T, ts *httptest.Server, creds auth.Credentials, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}
