// Package promote is the contract for scripted HA promote.
//
// The scripts are scripts/ha/promote.sh, scripts/ha/promote.ps1,
// scripts/cluster/promote.sh, and scripts/cluster/promote.ps1.
// There is no in-process failover controller. Promoting a standby does not
// turn on automatic multi-AZ failover.
//
// Fence: the old Postgres primary gets
// ALTER SYSTEM SET default_transaction_read_only = on. That file is not
// replicated, so the promoted node stays writable. A new session on the old
// primary cannot create or change tables. The storage-server renews its
// leader lease against that database; the renew fails and the process drops
// the lease immediately, so leaderGuard rejects HTTP writes without waiting
// for the previous TTL. Patroni labs demote the old primary instead
// (it re-enters recovery). See the exit codes below and the disaster-recovery
// runbook.
package promote

// Exit codes shared by the promote scripts. A re-run that finds this state
// already true exits ExitOK. Dry-run and plan-only also exit ExitOK.
const (
	// ExitOK means the new primary is writable and the old primary is fenced,
	// or that was already true.
	ExitOK = 0
	// ExitUsage means the arguments are wrong.
	ExitUsage = 1
	// ExitPrecheck means lag is above the limit, the standby cannot be
	// reached, or both nodes are writable primaries and the old one was not named.
	ExitPrecheck = 2
	// ExitFence means the old primary still accepted a write.
	ExitFence = 3
	// ExitPromote means pg_promote or the Patroni switchover failed.
	ExitPromote = 4
	// ExitPostcheck means the new primary did not leave recovery or did not accept a write.
	ExitPostcheck = 5
)
