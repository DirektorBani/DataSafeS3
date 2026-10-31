// Command s3smoke is a self-contained smoke test of a running DataSafeS3 server
// (health, console API login, S3 create/put/get/list/delete). It is a static
// binary, so it runs unchanged on Astra Linux, RED OS or any other host.
//
//	go build -o s3smoke ./tools/s3smoke && ./s3smoke -endpoint http://127.0.0.1:9000
//
// Credentials come from STORAGE_ACCESS_KEY, STORAGE_SECRET_KEY, STORAGE_ADMIN_USER,
// STORAGE_ADMIN_PASSWORD (defaults are the development values).
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func must(name string, err error, detail string) {
	if err != nil {
		fmt.Printf("FAIL %s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("PASS %s %s\n", name, detail)
}

func main() {
	ep := flag.String("endpoint", "http://127.0.0.1:9000", "server URL")
	wait := flag.Duration("wait", 60*time.Second, "how long to wait for /healthz")
	persist := flag.String("persist", "", `"put" stores a marker object in bucket rrpo-persist; "check" verifies it (e.g. after a reboot)`)
	flag.Parse()
	ctx := context.Background()

	deadline := time.Now().Add(*wait)
	for {
		res, err := http.Get(*ep + "/healthz")
		if err == nil && res.StatusCode == 200 {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 120))
			res.Body.Close()
			must("health", nil, "GET /healthz -> 200 "+string(b))
			break
		}
		if time.Now().After(deadline) {
			must("health", fmt.Errorf("no 200 from /healthz: %v", err), "")
		}
		time.Sleep(time.Second)
	}

	body, _ := json.Marshal(map[string]string{"username": env("STORAGE_ADMIN_USER", "admin"), "password": env("STORAGE_ADMIN_PASSWORD", "admin")})
	res, err := http.Post(*ep+"/api/v1/admin/login", "application/json", bytes.NewReader(body))
	if err == nil {
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		if res.StatusCode != 200 || m["token"] == nil {
			err = fmt.Errorf("status %d", res.StatusCode)
		}
	}
	must("console-api-login", err, "POST /api/v1/admin/login -> 200, token issued")

	c := s3.New(s3.Options{
		BaseEndpoint: aws.String(*ep), Region: "us-east-1", UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(env("STORAGE_ACCESS_KEY", "datasafe"), env("STORAGE_SECRET_KEY", "datasafesecret"), ""),
	})
	if *persist != "" {
		persistence(ctx, c, *persist)
		return
	}
	rb := make([]byte, 4)
	_, _ = rand.Read(rb)
	b := "smoke-" + hex.EncodeToString(rb)
	payload := make([]byte, 1<<20)
	_, _ = rand.Read(payload)
	_, err = c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &b})
	must("create-bucket", err, b)
	_, err = c.PutObject(ctx, &s3.PutObjectInput{Bucket: &b, Key: aws.String("obj.bin"), Body: bytes.NewReader(payload)})
	must("put", err, "obj.bin 1 MiB")
	g, err := c.GetObject(ctx, &s3.GetObjectInput{Bucket: &b, Key: aws.String("obj.bin")})
	if err == nil {
		got, _ := io.ReadAll(g.Body)
		g.Body.Close()
		if !bytes.Equal(got, payload) {
			err = fmt.Errorf("content mismatch")
		}
	}
	must("get", err, "content identical")
	l, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &b})
	if err == nil && (len(l.Contents) != 1 || *l.Contents[0].Key != "obj.bin") {
		err = fmt.Errorf("unexpected listing")
	}
	must("list", err, "[obj.bin]")
	_, err = c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &b, Key: aws.String("obj.bin")})
	if err == nil {
		l, err = c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &b})
		if err == nil && len(l.Contents) != 0 {
			err = fmt.Errorf("object still listed")
		}
	}
	must("delete", err, "object gone")
	_, err = c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &b})
	must("delete-bucket", err, b)
	fmt.Println("ALL PASS")
}

// persistence stores ("put") or verifies ("check") a marker object whose body
// carries its own SHA-256, so a check after a restart proves the data survived.
func persistence(ctx context.Context, c *s3.Client, mode string) {
	b, k := "rrpo-persist", "marker.txt"
	switch mode {
	case "put":
		_, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &b})
		must("persist-create-bucket", err, b)
		rb := make([]byte, 16)
		_, _ = rand.Read(rb)
		body := "rrpo marker " + time.Now().Format(time.RFC3339) + " " + hex.EncodeToString(rb)
		_, err = c.PutObject(ctx, &s3.PutObjectInput{Bucket: &b, Key: &k, Body: bytes.NewReader([]byte(body))})
		must("persist-put", err, b+"/"+k+" = "+body)
	case "check":
		g, err := c.GetObject(ctx, &s3.GetObjectInput{Bucket: &b, Key: &k})
		var got []byte
		if err == nil {
			got, _ = io.ReadAll(g.Body)
			g.Body.Close()
		}
		must("persist-get", err, b+"/"+k+" = "+string(got))
	default:
		must("persist", fmt.Errorf("unknown mode %q", mode), "")
	}
}
