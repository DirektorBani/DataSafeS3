package metadata

import "time"

const (
	SiteReplEventPut    = "put"
	SiteReplEventDelete = "delete"
	SiteReplTaskPending = "pending"
	SiteReplTaskDone    = "completed"
	SiteReplTaskFailed  = "failed"
)

type SiteReplicationPeer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	AccessKey string    `json:"access_key"`
	SecretKey string    `json:"secret_key,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type SiteReplicationRule struct {
	ID               string    `json:"id"`
	PeerID           string    `json:"peer_id,omitempty"`
	TrustedClusterID string    `json:"trusted_cluster_id,omitempty"`
	SourceBucket     string    `json:"source_bucket"`
	DestBucket       string    `json:"dest_bucket"`
	Direction        string    `json:"direction"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
}

type SiteReplicationTask struct {
	ID           string     `json:"id"`
	RuleID       string     `json:"rule_id"`
	Event        string     `json:"event"`
	SourceBucket string     `json:"source_bucket"`
	Key          string     `json:"key"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	Bytes        int64      `json:"bytes"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	NextAttempt  time.Time  `json:"next_attempt,omitempty"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
}

type SiteReplicationStatus struct {
	PendingCount    int       `json:"pending_count"`
	LastProcessedAt time.Time `json:"last_processed_at,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	LagSeconds      float64   `json:"lag_seconds"`
}
