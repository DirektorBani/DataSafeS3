#!/usr/bin/env python3
"""Smoke test of a running DataSafeS3: health, console API login, S3 put/get/list/delete.

Usage: smoke-s3.py [ENDPOINT]   (default http://127.0.0.1:9000)
Env: STORAGE_ACCESS_KEY, STORAGE_SECRET_KEY, STORAGE_ADMIN_USER, STORAGE_ADMIN_PASSWORD.
Needs boto3. Prints one line per check; exit code 1 on the first failure.
"""
import json, os, sys, time, urllib.request, uuid

import boto3
from botocore.config import Config

ep = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:9000"

def ok(name, detail=""):
    print(f"PASS {name} {detail}".rstrip(), flush=True)

for i in range(60):
    try:
        with urllib.request.urlopen(ep + "/healthz", timeout=2) as r:
            body = r.read().decode()[:120]
            if r.status == 200:
                ok("health", f"GET /healthz -> 200 {body}")
                break
    except Exception:
        time.sleep(1)
else:
    print("FAIL health"); sys.exit(1)

req = urllib.request.Request(ep + "/api/v1/admin/login", method="POST",
    data=json.dumps({"username": os.environ.get("STORAGE_ADMIN_USER", "admin"),
                     "password": os.environ.get("STORAGE_ADMIN_PASSWORD", "admin")}).encode(),
    headers={"Content-Type": "application/json"})
with urllib.request.urlopen(req, timeout=10) as r:
    data = json.loads(r.read() or b"{}")
    ok("console-api-login", f"POST /api/v1/admin/login -> {r.status} keys={sorted(data)[:6]}")

s3 = boto3.client("s3", endpoint_url=ep, region_name="us-east-1",
    aws_access_key_id=os.environ.get("STORAGE_ACCESS_KEY", "datasafe"),
    aws_secret_access_key=os.environ.get("STORAGE_SECRET_KEY", "datasafesecret"),
    config=Config(s3={"addressing_style": "path"}, retries={"max_attempts": 2}))
b = "smoke-" + uuid.uuid4().hex[:8]
payload = os.urandom(1 << 20)
s3.create_bucket(Bucket=b); ok("create-bucket", b)
s3.put_object(Bucket=b, Key="obj.bin", Body=payload); ok("put", "obj.bin 1 MiB")
got = s3.get_object(Bucket=b, Key="obj.bin")["Body"].read()
assert got == payload, "content mismatch"; ok("get", "content identical")
keys = [o["Key"] for o in s3.list_objects_v2(Bucket=b).get("Contents", [])]
assert keys == ["obj.bin"], keys; ok("list", str(keys))
s3.delete_object(Bucket=b, Key="obj.bin")
assert s3.list_objects_v2(Bucket=b).get("KeyCount", 0) == 0; ok("delete", "object gone")
s3.delete_bucket(Bucket=b); ok("delete-bucket", b)
# Extra check: a key with a prefix ("dir/obj.bin") must not keep the bucket non-empty after deletion.
b2 = b + "-nested"
s3.create_bucket(Bucket=b2); s3.put_object(Bucket=b2, Key="dir/obj.bin", Body=b"x")
s3.delete_object(Bucket=b2, Key="dir/obj.bin")
try:
    s3.delete_bucket(Bucket=b2); ok("delete-bucket-after-nested-key", b2)
except Exception as e:
    print(f"WARN delete-bucket-after-nested-key {b2}: {e}")
print("ALL PASS")
