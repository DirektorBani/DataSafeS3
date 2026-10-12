#!/usr/bin/env python3
"""Tiny path-style S3 client for the promote drill. Stdlib only."""
import datetime
import hashlib
import hmac
import os
import sys
import urllib.error
import urllib.parse
import urllib.request


def sign(method, url, body, access, secret, region):
    parts = urllib.parse.urlsplit(url)
    path = parts.path or "/"
    payload = body if body is not None else b""
    payload_hash = hashlib.sha256(payload).hexdigest()
    now = datetime.datetime.now(datetime.timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    date_stamp = now.strftime("%Y%m%d")
    host = parts.netloc
    canonical_headers = (
        f"host:{host}\n"
        f"x-amz-content-sha256:{payload_hash}\n"
        f"x-amz-date:{amz_date}\n"
    )
    signed_headers = "host;x-amz-content-sha256;x-amz-date"
    canonical = "\n".join([
        method,
        path,
        parts.query,
        canonical_headers,
        signed_headers,
        payload_hash,
    ])
    scope = f"{date_stamp}/{region}/s3/aws4_request"
    string_to_sign = "\n".join([
        "AWS4-HMAC-SHA256",
        amz_date,
        scope,
        hashlib.sha256(canonical.encode()).hexdigest(),
    ])

    def mac(key, msg):
        return hmac.new(key, msg.encode() if isinstance(msg, str) else msg, hashlib.sha256).digest()

    k = mac(("AWS4" + secret).encode(), date_stamp)
    k = mac(k, region)
    k = mac(k, "s3")
    k = mac(k, "aws4_request")
    sig = hmac.new(k, string_to_sign.encode(), hashlib.sha256).hexdigest()
    auth = (
        f"AWS4-HMAC-SHA256 Credential={access}/{scope}, "
        f"SignedHeaders={signed_headers}, Signature={sig}"
    )
    headers = {
        "Host": host,
        "x-amz-date": amz_date,
        "x-amz-content-sha256": payload_hash,
        "Authorization": auth,
    }
    return headers, payload


def call(method, url, body, access, secret, region, outfile=None):
    headers, payload = sign(method, url, body, access, secret, region)
    data = payload if method in ("PUT", "POST") else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read()
            code = resp.status
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        code = exc.code
    if outfile and code < 300:
        with open(outfile, "wb") as fh:
            fh.write(raw)
    sys.stdout.write(str(code) + "\n")
    if raw and code >= 300:
        sys.stderr.write(raw[:400].decode("utf-8", "replace") + "\n")
    return 0 if 200 <= code < 300 else 1


def main(argv):
    if len(argv) < 4:
        sys.stderr.write("usage: drill_s3.py mb|put|get BASE bucket [key] [file]\n")
        return 1
    op, base, bucket = argv[1], argv[2].rstrip("/"), argv[3]
    access = os.environ.get("AWS_ACCESS_KEY_ID", "datasafe")
    secret = os.environ.get("AWS_SECRET_ACCESS_KEY", "datasafesecret")
    region = os.environ.get("AWS_DEFAULT_REGION", "us-east-1")
    if op == "mb":
        return call("PUT", f"{base}/{bucket}", b"", access, secret, region)
    if op == "put":
        key, path = argv[4], argv[5]
        with open(path, "rb") as fh:
            body = fh.read()
        return call("PUT", f"{base}/{bucket}/{key}", body, access, secret, region)
    if op == "get":
        key, dest = argv[4], argv[5]
        return call("GET", f"{base}/{bucket}/{key}", b"", access, secret, region, outfile=dest)
    sys.stderr.write(f"unknown op {op}\n")
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
