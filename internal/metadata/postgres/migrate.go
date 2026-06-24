package postgres

import (
	"fmt"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// MigrationReport summarizes a BoltDB → PostgreSQL migration.
type MigrationReport struct {
	Buckets      int  `json:"buckets"`
	Objects      int  `json:"objects"`
	Users        int  `json:"users"`
	AccessKeys   int  `json:"access_keys"`
	Webhooks     int  `json:"webhooks"`
	AuditLogs    int  `json:"audit_logs"`
	Skipped      int  `json:"skipped"`
	Errors       int  `json:"errors"`
	IdempotentOK bool `json:"idempotent_ok"`
}

// MigrateBoltToPostgres copies all metadata from BoltDB into PostgreSQL.
func MigrateBoltToPostgres(boltPath, postgresDSN string) (MigrationReport, error) {
	var report MigrationReport
	bolt, err := metadata.OpenBolt(boltPath)
	if err != nil {
		return report, fmt.Errorf("open bolt: %w", err)
	}
	defer bolt.Close()

	pg, err := Open(postgresDSN, "")
	if err != nil {
		return report, fmt.Errorf("open postgres: %w", err)
	}
	defer pg.Close()

	_ = pg.EnsureDefaultTenant()

	for _, t := range mustList(bolt.ListTenants()) {
		_ = pg.PutTenant(t)
	}

	users, _ := bolt.ListUsers()
	for _, u := range users {
		if _, err := pg.GetUser(u.ID); err == nil {
			_ = pg.UpdateUser(u)
			report.Skipped++
			continue
		}
		if err := pg.PutUser(u); err != nil {
			report.Errors++
		} else {
			report.Users++
		}
	}

	for _, k := range mustList(bolt.ListAccessKeys()) {
		if err := pg.PutAccessKey(k); err != nil {
			report.Errors++
		} else {
			report.AccessKeys++
		}
	}

	buckets, _ := bolt.ListBuckets()
	for _, b := range buckets {
		bkey := b.EffectiveStorageKey()
		if _, err := pg.GetBucket(bkey); err == nil {
			_ = pg.UpdateBucket(b)
			report.Skipped++
		} else if err := pg.PutBucket(b); err != nil {
			report.Errors++
		} else {
			report.Buckets++
		}
	}

	for _, b := range buckets {
		bkey := b.EffectiveStorageKey()
		for _, o := range mustList(bolt.ListObjectVersions(bkey, "", 0)) {
			var err error
			if o.VersionID != "" {
				err = pg.PutObjectVersioned(o)
			} else {
				err = pg.PutObject(o)
			}
			if err != nil {
				report.Errors++
			} else {
				report.Objects++
			}
		}
	}

	for _, m := range mustList(bolt.ListMultipart("")) {
		_ = pg.PutMultipart(m)
	}
	if cfg, err := bolt.GetSystemConfig(); err == nil {
		_ = pg.PutSystemConfig(cfg)
	}
	for _, t := range mustList(bolt.ListTrash("")) {
		_ = pg.PutTrash(t)
	}
	for _, t := range mustList(bolt.ListConsoleTokens("")) {
		_ = pg.PutConsoleToken(t)
	}
	for _, h := range mustList(bolt.ListWebhooks()) {
		if err := pg.PutWebhook(h); err != nil {
			report.Errors++
		} else {
			report.Webhooks++
		}
	}
	if act, err := bolt.ListActivity(metadata.ActivityFilter{Period: "all", Limit: 100000}); err == nil {
		for _, a := range act.Events {
			if err := pg.AppendActivity(a); err != nil {
				report.Errors++
			} else {
				report.AuditLogs++
			}
		}
	}
	for _, c := range mustList(bolt.ListGatewayConnections()) {
		_ = pg.PutGatewayConnection(c)
	}
	for _, r := range mustList(bolt.ListReplicationRules()) {
		_ = pg.PutReplicationRule(r)
	}
	for _, t := range mustList(bolt.ListReplicationTasks("", 0)) {
		_ = pg.PutReplicationTask(t)
	}
	if stats, err := bolt.GetGatewayStats(); err == nil {
		_ = pg.PutGatewayStats(stats)
	}
	for _, j := range mustList(bolt.ListSyncJobs("", 0)) {
		_ = pg.PutSyncJob(j)
	}
	for _, f := range mustList(bolt.ListFederationClusters()) {
		_ = pg.PutFederationCluster(f)
	}
	for _, team := range mustList(bolt.ListTeams()) {
		_ = pg.PutTeam(team)
	}
	for _, u := range users {
		if u.TeamID != "" {
			_ = pg.AddUserTeam(u.ID, u.TeamID)
		}
		for _, tid := range mustList(bolt.ListUserTeamIDs(u.ID)) {
			_ = pg.AddUserTeam(u.ID, tid)
		}
	}

	for _, u := range users {
		for _, fav := range mustList(bolt.ListFavorites(u.ID)) {
			_ = pg.PutFavorite(fav)
		}
	}

	report.IdempotentOK = report.Errors == 0
	return report, nil
}

func mustList[T any](v []T, err error) []T {
	if err != nil {
		return nil
	}
	return v
}
