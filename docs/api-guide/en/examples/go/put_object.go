//go:build ignore

// Upload an object via AWS SDK v2 (SigV4) against DataSafeS3.
//
// Run from repository root:
//
//	go run docs/api-guide/en/examples/go/put_object.go my-bucket path/to/object.txt
//
// Environment: DATASAFE_ENDPOINT, DATASAFE_ACCESS_KEY, DATASAFE_SECRET_KEY, DATASAFE_REGION
package main

import (
	"bytes"
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: put_object.go <bucket> <key>")
	}
	bucket := os.Args[1]
	key := os.Args[2]
	body := []byte("hello from DataSafeS3 Go example\n")

	endpoint := env("DATASAFE_ENDPOINT", "http://127.0.0.1:9000")
	accessKey := env("DATASAFE_ACCESS_KEY", "datasafe")
	secretKey := env("DATASAFE_SECRET_KEY", "datasafesecret")
	region := env("DATASAFE_REGION", "us-east-1")

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		log.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded s3://%s/%s (%d bytes)", bucket, key, len(body))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
