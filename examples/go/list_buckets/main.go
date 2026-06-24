package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
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

	out, err := client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Buckets: %d\n", len(out.Buckets))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
