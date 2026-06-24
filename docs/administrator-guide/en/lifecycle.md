English | **[Русский](../ru/lifecycle.md)**

# Lifecycle policies

Automate object expiration and transition rules per bucket.

## Flow

```mermaid
flowchart LR
  upload[Object uploaded]
  rules[Lifecycle rules in metadata]
  job[Background evaluation]
  trash[Soft delete / expire]
  upload --> rules
  rules --> job
  job --> trash
```

## Configuration

Bucket settings → **Lifecycle** — define rules (prefix, days, action).

## Related

- **Trash** — soft-deleted objects in `.datasafe-trash`
- **Object Lock** — legal hold and retention (compliance)

## Full guide

[Dashboard and buckets](../../en/user-guide/02-dashboard-and-buckets.md)
