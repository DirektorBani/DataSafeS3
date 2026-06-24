#!/usr/bin/env python3
"""List DataSafeS3 buckets via S3-compatible API."""
import os
import boto3

endpoint = os.environ.get("DATASAFE_ENDPOINT", "http://127.0.0.1:9000")
client = boto3.client(
    "s3",
    endpoint_url=endpoint,
    aws_access_key_id=os.environ.get("DATASAFE_ACCESS_KEY", "datasafe"),
    aws_secret_access_key=os.environ.get("DATASAFE_SECRET_KEY", "datasafesecret"),
    region_name=os.environ.get("DATASAFE_REGION", "us-east-1"),
)
for b in client.list_buckets().get("Buckets", []):
    print(b["Name"])
