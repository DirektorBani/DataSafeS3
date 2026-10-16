package auth

// Dangerous actions require a fresh second factor when the bucket-admin MFA
// policy is on. The same list is published in access-model.md; a test fails
// if the two drift.
const (
	ActionCryptoShred         = "crypto_shred"
	ActionKeyRotate           = "key_rotate"
	ActionTrashPurge          = "trash_purge"
	ActionSharingPolicyChange = "sharing_policy_change"
	ActionGrantsChange        = "grants_change"
	ActionEncryptionDisable   = "encryption_disable"
	ActionObjectLockConfig    = "object_lock_config"
	ActionComplianceRetention = "compliance_retention"
	ActionBucketDelete        = "bucket_delete"
	ActionMFAReset            = "mfa_reset"
	ActionRevokeAllShares     = "revoke_all_shares"
	ActionAccessKeyCreate     = "access_key_create"
)

// DangerousActions is the canonical order used by the docs test.
var DangerousActions = []string{
	ActionCryptoShred,
	ActionKeyRotate,
	ActionTrashPurge,
	ActionSharingPolicyChange,
	ActionGrantsChange,
	ActionEncryptionDisable,
	ActionObjectLockConfig,
	ActionComplianceRetention,
	ActionBucketDelete,
	ActionMFAReset,
	ActionRevokeAllShares,
	ActionAccessKeyCreate,
}

// IsDangerousAction reports whether action is in DangerousActions.
func IsDangerousAction(action string) bool {
	for _, a := range DangerousActions {
		if a == action {
			return true
		}
	}
	return false
}
