English | **[Русский](../../ru/user-guide/03-klyuchi-i-kvoty.md)**

# 3. Access Keys, API Tokens, and Quotas

[← Buckets](02-dashboard-and-buckets.md) | [Table of contents](README.md) | Next: [Security →](04-security-and-profile.md)

---

## Access Section

In the left menu, select **Access**. There are two tabs:

1. **Access Keys** — for programs and S3-compatible applications.
2. **API Tokens** — for the console REST API.

---

## Access Keys (S3 Keys)

Keys are needed to connect applications: AWS CLI, backup tools, your own scripts.

### Default System Key

On installation, one key already exists (from server settings):

| Field | Value (local) |
|-------|---------------|
| Access Key ID | `datasafe` |
| Secret Key | `datasafesecret` |
| Endpoint | `http://localhost:9000` |
| Region | `us-east-1` |

> Do not publish the Secret Key. It is like a password to storage.

### Create Your Own Key

1. **Access Keys** tab → **Create key**.
2. The system shows **Access Key ID** and **Secret Key** — **copy the Secret immediately**; it will not be shown again.
3. Store the key in a secure place (password manager).

### Delete a Key

1. Find the key in the list.
2. **Delete** → confirm.

A deleted key stops working immediately.

---

## API Tokens (Console Tokens)

Tokens start with `ds_` and grant access to the admin console **REST API** (not S3 directly).

### Create a Token

1. **API Tokens** tab → **Create token**.
2. Specify a name, expiration (or no expiration), and scopes if needed.
3. Copy the token — it is shown only once.

### When an API Token Is Needed

- automation over HTTP (scripts, CI/CD);
- integrations that call `/api/v1/...` with the `Authorization: Bearer ds_...` header.

For everyday file work, a regular user only needs **Access Keys**.

---

## Usage (Usage and Quotas)

The **Usage** section in the menu shows how much space and how many objects you use.

### For a Regular User

- volume per bucket;
- growth charts.

### For an Administrator

- **system-wide** statistics;
- comparison with user limits.

### Quotas (Limits)

An administrator can set a per-user limit:

| Parameter | Description |
|-----------|-------------|
| Volume | For example 10 GB, 1 TB, or unlimited |
| Object count | Maximum number of files |

On **Dashboard** you see:

- **Used** — consumed;
- **Quota** — limit;
- **Remaining** — left.

When the limit is exceeded, **new uploads** are rejected until the administrator raises the quota or you delete files.

Per-bucket quotas are configured in bucket **Settings** (administrator).

---

## What's Next?

- [MFA and profile →](04-security-and-profile.md)
- [User and quota management (admin) →](05-administration.md)
