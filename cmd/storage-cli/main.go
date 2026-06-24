package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
)

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Region    string
}

func loadConfig() Config {
	return Config{
		Endpoint:  envOr("DATASAFE_ENDPOINT", "S3FORK_ENDPOINT", "http://127.0.0.1:9000"),
		AccessKey: envOr("DATASAFE_ACCESS_KEY", "S3FORK_ACCESS_KEY", "datasafe"),
		SecretKey: envOr("DATASAFE_SECRET_KEY", "S3FORK_SECRET_KEY", "datasafesecret"),
		Region:    envOr("DATASAFE_REGION", "S3FORK_REGION", "us-east-1"),
	}
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}
	cfg := loadConfig()
	switch args[0] {
	case "ls":
		if len(args) < 2 {
			if err := listBuckets(cfg); err != nil {
				fail(err)
			}
			return
		}
		bucket, prefix, err := parseS3URI(args[1])
		if err != nil {
			fail(err)
		}
		if err := listObjects(cfg, bucket, prefix); err != nil {
			fail(err)
		}
	case "mb":
		if len(args) < 2 {
			fail(fmt.Errorf("usage: storage-cli mb s3://bucket"))
		}
		bucket, _, err := parseS3URI(args[1])
		if err != nil {
			fail(err)
		}
		if err := makeBucket(cfg, bucket); err != nil {
			fail(err)
		}
	case "rb":
		if len(args) < 2 {
			fail(fmt.Errorf("usage: storage-cli rb s3://bucket"))
		}
		bucket, _, err := parseS3URI(args[1])
		if err != nil {
			fail(err)
		}
		if err := removeBucket(cfg, bucket); err != nil {
			fail(err)
		}
	case "cp":
		if len(args) < 3 {
			fail(fmt.Errorf("usage: storage-cli cp <src> <dst>"))
		}
		if err := copyCmd(cfg, args[1], args[2]); err != nil {
			fail(err)
		}
	case "rm":
		if len(args) < 2 {
			fail(fmt.Errorf("usage: storage-cli rm s3://bucket/key"))
		}
		bucket, key, err := parseS3URI(args[1])
		if err != nil {
			fail(err)
		}
		if key == "" {
			fail(fmt.Errorf("object key required"))
		}
		if err := deleteObject(cfg, bucket, key); err != nil {
			fail(err)
		}
	case "cat":
		if len(args) < 2 {
			fail(fmt.Errorf("usage: storage-cli cat s3://bucket/key"))
		}
		bucket, key, err := parseS3URI(args[1])
		if err != nil {
			fail(err)
		}
		if err := catObject(cfg, bucket, key); err != nil {
			fail(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`DataSafeS3 CLI (storage-cli) — Датасейф S3 command-line client

Usage:
  storage-cli <command> [args]

Commands:
  ls [s3://bucket/prefix]   List buckets or objects
  mb s3://bucket            Create bucket
  rb s3://bucket            Delete empty bucket
  cp <src> <dst>            Copy local<->s3 (s3://bucket/key)
  rm s3://bucket/key        Delete object
  cat s3://bucket/key       Print object to stdout

Environment:
  DATASAFE_ENDPOINT    Default http://127.0.0.1:9000 (alias: S3FORK_ENDPOINT)
  DATASAFE_ACCESS_KEY  Default datasafe (alias: S3FORK_ACCESS_KEY)
  DATASAFE_SECRET_KEY  Default datasafesecret (alias: S3FORK_SECRET_KEY)
  DATASAFE_REGION      Default us-east-1 (alias: S3FORK_REGION)
`)
}

func parseS3URI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("expected s3:// URI, got %q", uri)
	}
	path := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(path, "/", 2)
	bucket = parts[0]
	if len(parts) == 2 {
		key = parts[1]
	}
	return bucket, key, nil
}

func isS3(uri string) bool {
	return strings.HasPrefix(uri, "s3://")
}

func doRequest(cfg Config, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	url := strings.TrimRight(cfg.Endpoint, "/") + path
	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	creds := auth.Credentials{AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey}
	if err := auth.SignRequest(req, creds, cfg.Region, "s3", "UNSIGNED-PAYLOAD"); err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func listBuckets(cfg Config) error {
	resp, err := doRequest(cfg, http.MethodGet, "/", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("list buckets: HTTP %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	for _, line := range extractXMLValues(string(data), "Name") {
		fmt.Println(line)
	}
	return nil
}

func listObjects(cfg Config, bucket, prefix string) error {
	path := fmt.Sprintf("/%s?list-type=2&prefix=%s", bucket, prefix)
	resp, err := doRequest(cfg, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("list objects: HTTP %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	keys := extractXMLValues(string(data), "Key")
	sizes := extractXMLValues(string(data), "Size")
	for i, k := range keys {
		size := ""
		if i < len(sizes) {
			size = sizes[i]
		}
		fmt.Printf("%8s  %s\n", size, k)
	}
	return nil
}

func makeBucket(cfg Config, bucket string) error {
	resp, err := doRequest(cfg, http.MethodPut, "/"+bucket, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("create bucket: HTTP %d", resp.StatusCode)
	}
	fmt.Println("Bucket created:", bucket)
	return nil
}

func removeBucket(cfg Config, bucket string) error {
	resp, err := doRequest(cfg, http.MethodDelete, "/"+bucket, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete bucket: HTTP %d", resp.StatusCode)
	}
	fmt.Println("Bucket removed:", bucket)
	return nil
}

func deleteObject(cfg Config, bucket, key string) error {
	resp, err := doRequest(cfg, http.MethodDelete, "/"+bucket+"/"+key, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete object: HTTP %d", resp.StatusCode)
	}
	return nil
}

func catObject(cfg Config, bucket, key string) error {
	resp, err := doRequest(cfg, http.MethodGet, "/"+bucket+"/"+key, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("get object: HTTP %d", resp.StatusCode)
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

func copyCmd(cfg Config, src, dst string) error {
	if isS3(src) && !isS3(dst) {
		bucket, key, err := parseS3URI(src)
		if err != nil {
			return err
		}
		resp, err := doRequest(cfg, http.MethodGet, "/"+bucket+"/"+key, nil, nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("download: HTTP %d", resp.StatusCode)
		}
		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, resp.Body)
		return err
	}
	if !isS3(src) && isS3(dst) {
		bucket, key, err := parseS3URI(dst)
		if err != nil {
			return err
		}
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		st, _ := f.Stat()
		resp, err := doRequest(cfg, http.MethodPut, "/"+bucket+"/"+key, f, map[string]string{
			"Content-Type":   contentTypeFor(src),
			"Content-Length": fmt.Sprintf("%d", st.Size()),
		})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upload: HTTP %d %s", resp.StatusCode, body)
		}
		fmt.Printf("uploaded %s -> s3://%s/%s\n", src, bucket, key)
		return nil
	}
	return fmt.Errorf("cp supports local<->s3 only")
}

func contentTypeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func extractXMLValues(xml, tag string) []string {
	var out []string
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	for {
		i := strings.Index(xml, open)
		if i < 0 {
			break
		}
		xml = xml[i+len(open):]
		j := strings.Index(xml, close)
		if j < 0 {
			break
		}
		out = append(out, xml[:j])
		xml = xml[j+len(close):]
	}
	return out
}

func envOr(keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	def := keys[len(keys)-1]
	for _, k := range keys[:len(keys)-1] {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func init() {
	http.DefaultClient.Timeout = 60 * time.Second
	_ = bytes.NewReader(nil)
}
