package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
		return true
	}
	return false
}

func isBucketAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "BucketAlreadyOwnedByYou", "BucketAlreadyExists":
			return true
		}
	}
	return false
}

func (s *Server) ensureRemoteBucket(ctx context.Context, client *s3.Client, bucket, visibility string) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		if visibility == "public-read" {
			policy := publicReadBucketPolicy(bucket)
			_, _ = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
				Bucket: aws.String(bucket),
				Policy: aws.String(policy),
			})
		}
		return nil
	}
	if !isS3NotFound(err) {
		// HeadBucket may fail with 403 while CreateBucket/PutObject still work.
	}
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil && !isBucketAlreadyExists(err) {
		return err
	}
	if visibility == "public-read" {
		policy := publicReadBucketPolicy(bucket)
		_, perr := client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
			Bucket: aws.String(bucket),
			Policy: aws.String(policy),
		})
		if perr != nil {
			return perr
		}
	}
	return nil
}

func publicReadBucketPolicy(bucket string) string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect":    "Allow",
			"Principal": "*",
			"Action":    []string{"s3:GetObject"},
			"Resource":  []string{fmt.Sprintf("arn:aws:s3:::%s/*", bucket)},
		}},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func (s *Server) sourceBucketVisibility(bucket string) string {
	rec, err := s.meta.GetBucket(bucket)
	if err != nil || rec.Visibility == "" {
		return "private"
	}
	return rec.Visibility
}

func gatewayConnNotFoundErr(connID string) error {
	return fmt.Errorf("gateway connection %q not found (replication rule may reference a deleted connection)", connID)
}

func isLocalNotFound(err error) bool {
	return errors.Is(err, metadata.ErrNotFound)
}
