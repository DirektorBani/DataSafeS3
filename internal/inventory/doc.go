// Package inventory implements Admin-first storage inventory export (Governance Evidence Pack / A4).
//
// Manual CSV export works on Bolt and Postgres metadata. Optional dest-bucket write uses the
// normal PutObject path. This is not AWS S3 Inventory + Athena; see ADR-0004 and ops docs.
package inventory
