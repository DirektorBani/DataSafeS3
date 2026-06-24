package metadata

import "time"

// MetadataStore is the backend-agnostic metadata persistence contract.
// Implemented by BoltDB (*Store) and PostgreSQL (postgres.Store).
type MetadataStore interface {
	Close() error

	// Buckets
	PutBucket(rec BucketRecord) error
	GetBucket(name string) (BucketRecord, error)
	GetBucketByKey(storageKey string) (BucketRecord, error)
	ResolveBucket(scope BucketScope, name string) (BucketRecord, error)
	DeleteBucket(storageKey string) error
	ListBuckets() ([]BucketRecord, error)
	ListBucketsFiltered(filter BucketListFilter) ([]BucketRecord, error)
	UpdateBucket(rec BucketRecord) error
	SetBucketPolicy(bucket, policy string) error
	SetBucketLifecycle(bucket string, rules []LifecycleRule) error
	SetBucketTags(bucket string, tags map[string]string) error
	ListBucketsByTenant(tenantID string) ([]BucketRecord, error)

	// Bucket access grants (tenant admin)
	PutBucketAccessGrant(grant BucketAccessGrant) error
	ListBucketAccessGrants(bucketKey string) ([]BucketAccessGrant, error)
	DeleteBucketAccessGrant(bucketKey, userID string) error
	ReplaceBucketAccessGrants(bucketKey string, grants []BucketAccessGrant) error
	CountBucketAccessGrants(bucketKey string) (int, error)

	PutBucketPrefixAccessGrant(grant BucketPrefixAccessGrant) error
	ListBucketPrefixAccessGrants(bucketKey string) ([]BucketPrefixAccessGrant, error)
	ListUserPrefixAccessGrants(userID string) ([]BucketPrefixAccessGrant, error)
	DeleteBucketPrefixAccessGrant(bucketKey, userID, prefix string) error
	DeleteBucketPrefixAccessGrantsForUser(bucketKey, userID string) error
	ReplaceBucketPrefixAccessGrants(bucketKey string, grants []BucketPrefixAccessGrant) error
	CountBucketPrefixAccessGrants(bucketKey string) (int, error)

	PutUserNotification(rec UserNotificationRecord) error
	ListUserNotifications(userID string, limit int) ([]UserNotificationRecord, error)
	MarkUserNotificationRead(userID, id string) error
	MarkAllUserNotificationsRead(userID string) error
	CountUnreadNotifications(userID string) (int, error)
	TouchRecentItem(userID, bucket, prefix string) error
	ListRecentItems(userID string, limit int) ([]RecentItemRecord, error)

	// Objects
	PutObject(rec ObjectRecord) error
	PutObjectVersioned(rec ObjectRecord) error
	GetObject(bucket, key string) (ObjectRecord, error)
	GetObjectVersion(bucket, key, versionID string) (ObjectRecord, error)
	DeleteObject(bucket, key string) error
	DeleteObjectVersion(bucket, key, versionID string, versioningEnabled bool) error
	ListObjects(bucket, prefix string, maxKeys int) ([]ObjectRecord, error)
	ListObjectVersions(bucket, prefix string, maxKeys int) ([]ObjectRecord, error)
	ListObjectVersionIDs(bucket, key string) ([]string, error)
	ListObjectsPage(bucket, prefix, startAfter string, maxKeys int) ([]ObjectRecord, bool, string, error)
	SetObjectTags(bucket, key, versionID string, tags map[string]string) error
	UpdateObjectMeta(bucket, key, versionID string, meta map[string]string, contentType string) error
	SetObjectLegalHold(bucket, key, versionID string, hold bool) error
	SetObjectRetention(bucket, key, versionID string, until time.Time) error
	TotalObjectBytes() (int64, error)
	CountObjects() (int, error)
	BucketObjectCount(bucket string) (int, error)
	BucketTotalSize(bucket string) (int64, error)

	// Multipart
	PutMultipart(rec MultipartRecord) error
	GetMultipart(uploadID string) (MultipartRecord, error)
	DeleteMultipart(uploadID string) error
	ListMultipart(bucket string) ([]MultipartRecord, error)

	// Access keys
	PutAccessKey(rec AccessKeyRecord) error
	GetAccessKey(accessKey string) (AccessKeyRecord, error)
	ListAccessKeys() ([]AccessKeyRecord, error)
	DeleteAccessKey(accessKey string) error

	// Users
	PutUser(rec UserRecord) error
	GetUser(id string) (UserRecord, error)
	GetUserByUsername(username string) (UserRecord, error)
	UpdateUser(rec UserRecord) error
	DeleteUser(id string) error
	ListUsers() ([]UserRecord, error)
	CountUsers() (int, error)

	// Teams
	PutTeam(rec TeamRecord) error
	GetTeam(id string) (TeamRecord, error)
	ListTeams() ([]TeamRecord, error)
	DeleteTeam(id string) error
	AddUserTeam(userID, teamID string) error
	RemoveUserTeam(userID, teamID string) error
	ListUserTeamIDs(userID string) ([]string, error)

	// Tenants
	EnsureDefaultTenant() error
	PutTenant(rec TenantRecord) error
	GetTenant(id string) (TenantRecord, error)
	ListTenants() ([]TenantRecord, error)
	DeleteTenant(id string) error

	// System config & trash
	GetSystemConfig() (SystemConfig, error)
	PutSystemConfig(cfg SystemConfig) error
	PutTrash(rec TrashRecord) error
	GetTrash(id string) (TrashRecord, error)
	DeleteTrash(id string) error
	ListTrash(bucketFilter string) ([]TrashRecord, error)

	// API tokens
	PutConsoleToken(rec ConsoleTokenRecord) error
	GetConsoleToken(id string) (ConsoleTokenRecord, error)
	FindConsoleTokenByHash(hash string) (ConsoleTokenRecord, error)
	ListConsoleTokens(userID string) ([]ConsoleTokenRecord, error)
	DeleteConsoleToken(id string) error

	// Webhooks
	PutWebhook(rec WebhookRecord) error
	GetWebhook(id string) (WebhookRecord, error)
	ListWebhooks() ([]WebhookRecord, error)
	DeleteWebhook(id string) error
	PutWebhookDelivery(rec WebhookDeliveryRecord) error
	GetWebhookDelivery(id string) (WebhookDeliveryRecord, error)
	ListWebhookDeliveries(webhookID string, limit int) ([]WebhookDeliveryRecord, error)

	// Activity / audit
	AppendActivity(rec ActivityRecord) error
	ListActivity(f ActivityFilter) (ActivityListResult, error)

	// Usage
	AddUsageBytes(upload, download int64) error
	GetUsageCounters() (UsageCounters, error)
	PutUsageSnapshot(snap UsageSnapshot) error
	ListUsageSnapshots(days int) ([]UsageSnapshot, error)
	BucketUsageStats(filter BucketListFilter) ([]BucketUsage, error)
	OwnerUsage(owner string) (objectCount int, totalBytes int64, err error)

	// Search & favorites
	Search(query string, ownerFilter string, includeUsers bool, offset, limit int) ([]SearchResult, int, error)
	PutFavorite(rec FavoriteRecord) error
	ListFavorites(userID string) ([]FavoriteRecord, error)
	GetFavorite(userID, id string) (FavoriteRecord, error)
	DeleteFavorite(userID, id string) error
	FindFavorite(userID, favType, bucket, prefix string) (FavoriteRecord, error)

	// Gateway & replication
	PutGatewayConnection(rec GatewayConnection) error
	GetGatewayConnection(id string) (GatewayConnection, error)
	ListGatewayConnections() ([]GatewayConnection, error)
	DeleteGatewayConnection(id string) error
	PutReplicationRule(rec ReplicationRule) error
	GetReplicationRule(id string) (ReplicationRule, error)
	ListReplicationRules() ([]ReplicationRule, error)
	DeleteReplicationRule(id string) error
	ListReplicationRulesForBucket(bucket string) ([]ReplicationRule, error)
	PutReplicationTask(rec ReplicationTask) error
	GetReplicationTask(id string) (ReplicationTask, error)
	ListReplicationTasks(status string, limit int) ([]ReplicationTask, error)
	ListDueReplicationTasks(limit int, now time.Time) ([]ReplicationTask, error)
	CountPendingReplicationTasks() (int, time.Time, error)
	GetGatewayStats() (GatewayStats, error)
	PutGatewayStats(stats GatewayStats) error
	UpdateGatewayStats(fn func(*GatewayStats)) error
	AddReplicationError(rec ReplicationErrorRecord) error
	ListReplicationErrors(limit int) ([]ReplicationErrorRecord, error)
	ClearReplicationErrors() error
	RetryFailedReplicationTasks() (int, error)
	CountBrokenReplicationRules() (int, error)
	PutSyncJob(rec SyncJob) error
	GetSyncJob(id string) (SyncJob, error)
	ListSyncJobs(ruleID string, limit int) ([]SyncJob, error)

	// Federation
	PutFederationCluster(rec FederationCluster) error
	GetFederationCluster(id string) (FederationCluster, error)
	ListFederationClusters() ([]FederationCluster, error)
	DeleteFederationCluster(id string) error

	// Shared links
	PutSharedLink(rec SharedLinkRecord) error
	GetSharedLink(id string) (SharedLinkRecord, error)
	GetSharedLinkByToken(token string) (SharedLinkRecord, error)
	ListSharedLinks(bucket, key string) ([]SharedLinkRecord, error)
	DeleteSharedLink(id string) error
	IncrementSharedLinkDownload(id string) (SharedLinkRecord, error)

	// Tenant members
	PutTenantMember(rec TenantMemberRecord) error
	GetTenantMember(tenantID, userID string) (TenantMemberRecord, error)
	ListTenantMembers(tenantID string) ([]TenantMemberRecord, error)
	ListUserTenants(userID string) ([]TenantMemberRecord, error)
	UpdateTenantMemberRole(tenantID, userID, role string) error
	DeleteTenantMember(tenantID, userID string) error

	// Tenant groups
	PutTenantGroup(rec TenantGroupRecord) error
	GetTenantGroup(id string) (TenantGroupRecord, error)
	ListTenantGroups(tenantID string) ([]TenantGroupRecord, error)
	DeleteTenantGroup(id string) error
	CountTenantGroups(tenantID string) (int, error)
	ReplaceTenantGroupBuckets(groupID string, bucketKeys []string) error
	ListTenantGroupBuckets(groupID string) ([]string, error)
	ListTenantGroupBucketKeys(tenantID string) ([]string, error)
	ReplaceUserTenantGroups(tenantID, userID string, groupIDs []string) error
	ListUserTenantGroupIDs(tenantID, userID string) ([]string, error)
	ListTenantGroupMembers(groupID string) ([]string, error)
	ListUserGroupBucketAccess(userID string) ([]UserGroupBucketAccess, error)
	RemoveUserFromTenantGroups(tenantID, userID string) error

	// HA / Postgres monitoring
	ReplicationLagSeconds() (float64, bool)
}

// Compile-time check: BoltDB Store implements MetadataStore.
var _ MetadataStore = (*Store)(nil)
