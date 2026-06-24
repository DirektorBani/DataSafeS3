English | **[Русский](../../ru/user-guide/05-administraciya.md)**

# 5. Administration

[← Security](04-security-and-profile.md) | [Table of contents](README.md) | Next: [Gateway →](06-gateway-and-minio.md)

> The **Administration** section is visible only to users with the **administrator** role.

---

## Users

**Administration → Users**

### Create a User

1. **Create user**.
2. Fill in: login, email, password, **role** (user / operator / administrator).
3. Save.

### Roles When Creating

| Role | When to Assign |
|------|----------------|
| user | Regular access to own data |
| operator | Support staff, access to all buckets without admin settings |
| administrator | Full system management |

### User Quota

1. In the user row, select **Set quota**.
2. Specify volume (MB / GB / TB) or **Unlimited**.
3. Optionally — object count limit.
4. Save.

### Password Reset

**Reset password** → new password → share with the user through a secure channel.

### Deletion

**Delete** — the user and their keys stop working. Buckets may remain (depends on ownership).

---

## Teams

**Teams** let multiple users work with **shared buckets**.

How it works:

- a bucket can be linked to a **team**;
- all team members see and use that bucket;
- a regular user does **not** see other users' personal buckets.

> A dedicated **Teams** page in the console is still in development. Team membership and bucket assignment are configured at the data layer (migration, API). Contact your technical administrator or see [docs/context/](../context/).

---

## Tenants

**Administration → Tenants**

Used when one DataSafeS3 installation serves **multiple organizations** (multi-tenant):

1. **Create tenant** — organization name.
2. **Add member** — select a user and role in the tenant:
   - **tenant_admin** — full access to the tenant's buckets (read and write);
   - **member** — read and write objects in the tenant's buckets;
   - **viewer** — read only (upload and delete blocked).
3. Buckets with the same `tenant_id` as the tenant are visible to all its members according to role.
4. Users **outside** the tenant do not see other tenants' buckets, even if they know the name.

When a bucket is created, the tenant is assigned automatically from the owner's profile.

---

## Settings (System Settings)

**Administration → Settings**

| Section | What Is Configured |
|---------|-------------------|
| **Trash** | Soft delete, trash retention (1–3650 days) |
| **MFA** | Mandatory MFA for administrators |
| **LDAP** | Sign-in through corporate directory (Active Directory, etc.) |
| **OIDC / SSO** | Sign-in through Keycloak, Authentik, and others |
| **Cluster** | Distributed mode (see [chapter 8](08-federation-and-cluster.md)) |

### Per-Bucket Settings

Open a bucket → **Settings**:

| Tab | Purpose |
|-----|---------|
| General | Description, storage class (Hot/Warm/Cold) |
| Versioning | Enable object versions |
| Object Lock | WORM — deletion blocked until retention expires |
| Lifecycle | Auto-delete old files / versions |
| Visibility | Storage type: **Private** (authorized only) or **Public read only** (anonymous read via S3) |
| Quotas | Per-bucket limit |

---

## Policies (Access Policies)

**Administration → Policies**

Policies describe **who can do what** with a bucket (similar to IAM in AWS):

1. Select a bucket.
2. Use the **visual editor** or the **JSON** tab.
3. Save.

Example: allow read-only (`GetObject`, `ListBucket`) for a specific user.

---

## Activity (Activity Log)

**Administration → Activity**

- who created or deleted a bucket or file, and when;
- sign-ins, settings changes;
- filters by user, action, date.

Useful for audit and incident investigation.

---

## Webhooks (Notifications)

**Administration → Webhooks**

Sends HTTP requests to your URL on events:

| Event | When It Fires |
|-------|---------------|
| ObjectCreated | File uploaded |
| ObjectDeleted | File deleted |
| BucketCreated / BucketDeleted | Bucket created / deleted |
| UserCreated | User created |

### Configure a Webhook

1. **Create webhook**.
2. Recipient URL, event list, optional headers.
3. Save.
4. In **Delivery log**, view successful and failed deliveries; you can **Retry**.

---

## What's Next?

- [Gateway and replication →](06-gateway-and-minio.md)
- [Federation and Cluster →](08-federation-and-cluster.md)
