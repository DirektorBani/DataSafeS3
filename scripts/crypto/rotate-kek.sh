#!/usr/bin/env bash
# Skeleton: KEK rotation procedure for DataSafeS3 field encryption.
# Implement operational steps manually until admin rewrap API ships (see field-encryption-1.0.3-tz.md).
#
# Usage (after reviewing each step):
#   OLD_KEK_ID=kek-v1 NEW_KEK_ID=kek-20260630-a ./scripts/crypto/rotate-kek.sh
#
set -euo pipefail

: "${OLD_KEK_ID:?Set OLD_KEK_ID to the current active kek_id}"
: "${NEW_KEK_ID:?Set NEW_KEK_ID for the new key (generate with generate-kek.sh first)}"

cat <<EOF
=== KEK rotation checklist (manual) ===

1. Generate new keypair:
     DATASAFE_KEK_ID=${NEW_KEK_ID} ./scripts/crypto/generate-kek.sh

2. Register public key in encryption_key_registry (Postgres example):
     -- INSERT new row with is_active=false first, then flip active in transaction
     -- UPDATE old row: is_active=false, rotated_at=NOW()

3. Update process env (both keys for decrypt during transition):
     STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=${NEW_KEK_ID}
     STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=<new private b64>
     STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS='{"${OLD_KEK_ID}":"<old b64>","${NEW_KEK_ID}":"<new b64>"}'

4. Rolling restart all storage-server instances.

5. Lazy re-encrypt: touch records via normal API (update access keys, gateway, settings)
   OR call POST /api/v1/admin/encryption/rewrap when implemented.

6. Verify security-status: legacy_plaintext_fields_estimate → 0.

7. Remove ${OLD_KEK_ID} from STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS env.

8. After grace period, retire old key:
     UPDATE encryption_key_registry SET retired_at=NOW() WHERE kek_id='${OLD_KEK_ID}';

9. Securely delete old PEM files under data/keys/ (local dev only).

See docs/operations-guide/en/field-encryption.md and scripts/crypto/README.md for full procedure.
See docs/specs/field-encryption-1.0.3-tz.md §5.4 for full design.
EOF
