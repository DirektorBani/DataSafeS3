package postgres

import "context"

func (s *Store) ReplicationLagSeconds() (float64, bool) {
	ctx := context.Background()
	var lag *float64
	err := s.pool.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM replay_lag)
		FROM pg_stat_replication
		WHERE state = 'streaming'
		ORDER BY replay_lag DESC NULLS LAST
		LIMIT 1`).Scan(&lag)
	if err != nil || lag == nil {
		var recovery bool
		_ = s.pool.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovery)
		if !recovery {
			return 0, true
		}
		var replayLag *float64
		_ = s.pool.QueryRow(ctx, `
			SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))`).Scan(&replayLag)
		if replayLag != nil {
			return *replayLag, true
		}
		return 0, true
	}
	return *lag, true
}
