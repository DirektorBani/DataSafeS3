English | **[Русский](../../ru/user-guide/02-dashbord-i-bakety.md)**

# 2. Dashboard and Buckets

[← Introduction](01-introduction-and-login.md) | [Table of contents](README.md) | Next: [Keys and quotas →](03-keys-and-quotas.md)

---

## Dashboard

After sign-in, **Dashboard** opens — a brief summary:

| Card | What It Shows |
|------|---------------|
| Buckets | How many buckets you have |
| Objects | How many files (objects) |
| Storage | How much space is used |
| Used / Quota / Remaining | Your limit (if the administrator set a quota) |

Below that is a chart of data volume growth over recent time.

![Dashboard home page](../../user-guide/images/dashboard.png)

> The administrator sees system-wide statistics. A regular user sees only their own buckets.

---

## Bucket List

1. In the left menu, select **Buckets**.
2. A table of all buckets available to you is displayed.

![Bucket list](../../user-guide/images/buckets.png)

### Create a Bucket

1. Click **Create bucket** (or an equivalent button).
2. Enter a **name** — Latin letters, digits, and hyphens only; no spaces.
3. Confirm creation.

### Delete a Bucket

1. Find the bucket in the list.
2. Click delete and confirm.

> The bucket must be **empty**, otherwise deletion may be blocked.

---

## Object Browser (Files)

Click a bucket name to open the file view inside it.

![Object browser inside a bucket](../../user-guide/images/bucket-detail.png)

### Upload Files

**Method 1 — drag and drop**

1. Drag files or folders into the list area.

**Method 2 — button**

1. Click **Upload**.
2. Select files on your computer.

### Folders

- To create a "folder", upload a file with a path such as `reports/2026/january/file.pdf`, or create an empty folder through the UI (if available).
- Navigate folders by clicking the name; breadcrumbs at the top show the current path.

### Download a File

- Click the file or use the **Download** action in the row menu.

### Delete a File

1. Select a file (or several — bulk selection).
2. Click **Delete**.
3. Confirm.

If **Trash** is enabled, the file goes to the trash instead of being permanently deleted (see below).

### File Metadata

When you select a file, a panel may open on the right with information:

- size, date, type;
- tags;
- **Legal Hold** (legal lock — deletion blocked);
- retention period (WORM), if configured on the bucket.

---

## Tabs Inside a Bucket

| Tab | Purpose |
|-----|---------|
| **Objects** | List of files and folders |
| **Versions** | Version history (if versioning is enabled) |
| **Settings** | Bucket settings: versions, lifecycle, quotas |
| **Trash** | Trash for deleted objects |

---

## Versioning

If the administrator enabled **versioning** for the bucket:

- each new upload of a file with the same name creates a **new version**;
- older versions can be viewed on the **Versions** tab;
- deletion may create a delete marker while data remains in older versions.

Enable: bucket **Settings** → **Versioning** → turn on.

---

## Trash

With soft delete enabled, deleted files go to the `.datasafe-trash` service bucket.

1. Open the bucket → **Trash** tab.
2. Find the deleted object.
3. **Restore** — restore to the original bucket.
4. **Purge** — delete permanently.

Trash retention is set by the administrator in **Settings → Trash** (1 to 3650 days).

---

## Share a File (Presigned URL)

A temporary link grants access to the file without signing in to the console.

1. In the object list, open the action menu for the file → **Share** (or equivalent).
2. Choose the link expiration (for example 1 hour, 24 hours).
3. Copy the generated **link**.
4. Send the link to the recipient.

> After expiration, the link stops working.

---

## Search

The console has **global search** across buckets and objects — enter a name or part of a name in the search field.

---

## Favorites

You can pin frequently used buckets or folders to favorites for quick access.

---

## What's Next?

- [Access keys and quotas →](03-keys-and-quotas.md)
- [Bucket administration (policies, lifecycle) →](05-administration.md)
