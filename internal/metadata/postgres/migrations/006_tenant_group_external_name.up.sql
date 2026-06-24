-- Optional IdP/LDAP group name mapped to this tenant group (case-insensitive match on login).
ALTER TABLE tenant_groups ADD COLUMN IF NOT EXISTS external_name TEXT NOT NULL DEFAULT '';
