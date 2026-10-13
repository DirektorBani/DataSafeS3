package events

import "encoding/json"

// Notification schema_version written in the datasafe block.
const NotificationSchemaVersion = 1

// KafkaRecordKey is the producer record key: tenant/bucket/key.
// Object keys may contain slashes. Consumers should read the JSON body to
// recover the object key. The record key exists so one object stays on one
// partition.
func KafkaRecordKey(tenantID, bucket, key string) string {
	return tenantID + "/" + bucket + "/" + key
}

type notificationMessage struct {
	Records  []notificationRecord `json:"Records"`
	DataSafe datasafeBlock        `json:"datasafe"`
}

type notificationRecord struct {
	EventVersion string    `json:"eventVersion"`
	EventSource  string    `json:"eventSource"`
	EventName    string    `json:"eventName"`
	EventTime    string    `json:"eventTime"`
	S3           s3Section `json:"s3"`
}

type s3Section struct {
	Bucket s3Bucket `json:"bucket"`
	Object s3Object `json:"object"`
}

type s3Bucket struct {
	Name string `json:"name"`
}

type s3Object struct {
	Key       string `json:"key"`
	Size      int64  `json:"size"`
	ETag      string `json:"eTag"`
	VersionID string `json:"versionId"`
}

type shareBlock struct {
	LinkID   string `json:"link_id,omitempty"`
	Actor    string `json:"actor,omitempty"`
	ClientIP string `json:"client_ip,omitempty"`
}

type datasafeBlock struct {
	TenantID      string      `json:"tenant_id"`
	EventID       string      `json:"event_id"`
	SchemaVersion int         `json:"schema_version"`
	Share         *shareBlock `json:"share,omitempty"`
}

// NotificationJSON is the Kafka body. It is not the webhook or NATS body.
// eventName maps ObjectCreated to s3:ObjectCreated:Put (copy is not a
// separate stored type), ObjectDeleted to s3:ObjectRemoved:Delete, and
// MultipartCompleted to s3:ObjectCreated:CompleteMultipartUpload. Other
// types use a datasafe: prefix. eTag and size come from the stored payload.
// Put, copy, and multipart copy them from the committed object at insert.
// versionId is the outbox version id, also copied from that object at insert.
func NotificationJSON(e Event) ([]byte, error) {
	when := e.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	msg := notificationMessage{
		Records: []notificationRecord{{
			EventVersion: "2.2",
			EventSource:  "datasafe:s3",
			EventName:    notificationEventName(e.Type),
			EventTime:    when,
			S3: s3Section{
				Bucket: s3Bucket{Name: e.Bucket},
				Object: s3Object{
					Key:       e.Key,
					Size:      payloadInt64(e.Payload, "size"),
					ETag:      payloadString(e.Payload, "etag", "eTag", "ETag"),
					VersionID: e.VersionID,
				},
			},
		}},
		DataSafe: datasafeBlock{
			TenantID:      e.TenantID,
			EventID:       e.ID,
			SchemaVersion: NotificationSchemaVersion,
			Share:         shareBlockFrom(e),
		},
	}
	return json.Marshal(msg)
}

func notificationEventName(eventType string) string {
	switch eventType {
	case "ObjectCreated":
		return "s3:ObjectCreated:Put"
	case "ObjectDeleted":
		return "s3:ObjectRemoved:Delete"
	case "MultipartCompleted":
		return "s3:ObjectCreated:CompleteMultipartUpload"
	case "BucketCreated":
		return "s3:BucketCreated"
	case "BucketDeleted":
		return "s3:BucketRemoved"
	case "UserCreated":
		return "datasafe:UserCreated"
	case "ShareCreated":
		return "datasafe:Share:Created"
	case "ShareRevoked":
		return "datasafe:Share:Revoked"
	case "ShareDownloaded":
		return "datasafe:Share:Downloaded"
	case "ShareExpired":
		return "datasafe:Share:Expired"
	case "ShareLimitReached":
		return "datasafe:Share:LimitReached"
	case "ShareBlocked":
		return "datasafe:Share:Blocked"
	case "PresignIssued":
		return "datasafe:Presign:Issued"
	case "SharingPolicyChanged":
		return "datasafe:Sharing:PolicyChanged"
	case "TestEvent":
		return "datasafe:TestEvent"
	default:
		return "datasafe:" + eventType
	}
}

func shareBlockFrom(e Event) *shareBlock {
	switch e.Type {
	case "ShareCreated", "ShareRevoked", "ShareDownloaded", "ShareExpired", "ShareLimitReached", "ShareBlocked", "PresignIssued", "SharingPolicyChanged":
	default:
		return nil
	}
	block := &shareBlock{
		LinkID:   payloadString(e.Payload, "link_id"),
		Actor:    payloadString(e.Payload, "actor"),
		ClientIP: payloadString(e.Payload, "client_ip"),
	}
	if block.LinkID == "" && block.Actor == "" && block.ClientIP == "" {
		return nil
	}
	return block
}

func payloadString(p map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := p[k].(string); ok {
			return s
		}
	}
	return ""
}

func payloadInt64(p map[string]any, key string) int64 {
	switch v := p[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}
