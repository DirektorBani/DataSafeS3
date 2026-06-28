package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

const (
	defaultReplWorkerInterval = 2 * time.Second
	defaultReplMaxRetries     = 5
	defaultReplBatchSize      = 20
)

func (s *Server) wireReplicationHooks() {
	s.svc.OnObjectEvent = func(event, bucket, key string) {
		s.enqueueObjectReplication(event, bucket, key)
		var size int64
		if obj, err := s.meta.GetObject(bucket, key); err == nil {
			size = obj.Size
		}
		s.svc.EmitBucketNotification(bucket, event, key, size)
	}
	s.svc.OnBucketNotification = s.deliverBucketNotification
}

func (s *Server) deliverBucketNotification(webhookURL, event, bucket, key string, size int64) {
	if err := urlpolicy.ValidateOutboundURL(webhookURL, urlpolicy.DefaultOptions()); err != nil {
		return
	}
	payload := map[string]any{
		"event":  event,
		"bucket": bucket,
		"key":    key,
		"size":   size,
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DataSafe-Event", event)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (s *Server) enqueueObjectReplication(event, bucket, key string) {
	if bucket == metadata.TrashBucketName || key == "" {
		return
	}
	logicalBucket := bucket
	if rec, err := s.meta.GetBucketByKey(bucket); err == nil {
		logicalBucket = rec.Name
	} else if rec, err := s.meta.GetBucket(bucket); err == nil {
		logicalBucket = rec.Name
	}
	rules, err := s.meta.ListReplicationRulesForBucket(logicalBucket)
	if err != nil || len(rules) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, rule := range rules {
		task := metadata.ReplicationTask{
			ID:           randomID(),
			RuleID:       rule.ID,
			Event:        event,
			SourceBucket: logicalBucket,
			Key:          key,
			Status:       metadata.ReplTaskPending,
			CreatedAt:    now,
			NextAttempt:  now,
		}
		_ = s.meta.PutReplicationTask(task)
	}
}

func (s *Server) runReplicationWorker(ctx context.Context) {
	interval := defaultReplWorkerInterval
	if v := os.Getenv("STORAGE_GATEWAY_WORKER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processReplicationQueueOnce(ctx)
		}
	}
}

func (s *Server) runReplicationFullSyncWorker(ctx context.Context) {
	interval := time.Hour
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STORAGE_GATEWAY_CONTINUOUS_SYNC")), "true") {
		interval = 5 * time.Minute
	}
	if v := os.Getenv("STORAGE_GATEWAY_FULL_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rules, err := s.meta.ListReplicationRules()
			if err != nil {
				continue
			}
			for _, rule := range rules {
				if !rule.Enabled {
					continue
				}
				s.enqueueFullBucketScan(rule)
			}
		}
	}
}

func (s *Server) enqueueFullBucketScan(rule metadata.ReplicationRule) {
	objs, err := s.meta.ListObjects(s.storageKeyForLogicalBucket(rule.SourceBucket), "", 0)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, obj := range objs {
		if obj.IsDeleteMarker || obj.Size == 0 {
			continue
		}
		task := metadata.ReplicationTask{
			ID:           randomID(),
			RuleID:       rule.ID,
			Event:        metadata.ReplEventPut,
			SourceBucket: rule.SourceBucket,
			Key:          obj.Key,
			Status:       metadata.ReplTaskPending,
			CreatedAt:    now,
			NextAttempt:  now,
		}
		_ = s.meta.PutReplicationTask(task)
	}
}

// ProcessReplicationQueueOnce drains due replication tasks (for tests).
func (s *Server) ProcessReplicationQueueOnce(ctx context.Context) {
	s.processReplicationQueueOnce(ctx)
}

func (s *Server) processReplicationQueueOnce(ctx context.Context) {
	now := time.Now().UTC()
	tasks, err := s.meta.ListDueReplicationTasks(defaultReplBatchSize, now)
	if err != nil || len(tasks) == 0 {
		return
	}
	maxRetries := defaultReplMaxRetries
	if v := os.Getenv("STORAGE_GATEWAY_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxRetries = n
		}
	}
	for _, task := range tasks {
		bytes, err := s.executeReplicationTask(ctx, task)
		done := time.Now().UTC()
		task.ProcessedAt = &done
		if err != nil {
			task.Attempts++
			task.Error = err.Error()
			if task.Attempts >= maxRetries {
				task.Status = metadata.ReplTaskFailed
				_ = s.meta.UpdateGatewayStats(func(st *metadata.GatewayStats) {
					st.ReplicationErrors++
				})
				_ = s.meta.AddReplicationError(metadata.ReplicationErrorRecord{
					ID:           randomID(),
					TaskID:       task.ID,
					RuleID:       task.RuleID,
					Event:        task.Event,
					SourceBucket: task.SourceBucket,
					Key:          task.Key,
					Message:      err.Error(),
					CreatedAt:    done,
				})
				s.logActivityAs("system", "", metadata.ActionGatewayReplFailed, "gateway",
					fmt.Sprintf("%s/%s: %s", task.SourceBucket, task.Key, err.Error()))
			} else {
				backoff := time.Duration(math.Min(float64(time.Second)*math.Pow(2, float64(task.Attempts)), float64(60*time.Second)))
				task.NextAttempt = now.Add(backoff)
				task.Status = metadata.ReplTaskPending
			}
		} else {
			task.Status = metadata.ReplTaskCompleted
			task.Bytes = bytes
			task.Error = ""
			_ = s.meta.UpdateGatewayStats(func(st *metadata.GatewayStats) {
				st.BytesReplicated += bytes
				st.TasksCompletedTotal++
				st.LastProcessedAt = done
			})
			if bytes > 0 {
				s.logActivityAs("system", "", metadata.ActionGatewayReplicated, "gateway",
					fmt.Sprintf("%s/%s (%d bytes)", task.SourceBucket, task.Key, bytes))
			}
		}
		_ = s.meta.PutReplicationTask(task)
	}
}

func (s *Server) executeReplicationTask(ctx context.Context, task metadata.ReplicationTask) (int64, error) {
	rule, err := s.meta.GetReplicationRule(task.RuleID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return 0, fmt.Errorf("replication rule %q not found", task.RuleID)
		}
		return 0, err
	}
	if !rule.Enabled {
		return 0, fmt.Errorf("rule disabled")
	}
	conn, err := s.meta.GetGatewayConnection(rule.DestConnection)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return 0, gatewayConnNotFoundErr(rule.DestConnection)
		}
		return 0, err
	}
	client, err := s.gatewayS3Client(conn)
	if err != nil {
		return 0, err
	}
	remoteKey := task.Key
	switch task.Event {
	case metadata.ReplEventDelete:
		return s.replicateDeleteObject(ctx, client, rule, remoteKey)
	default:
		return s.replicatePutObject(ctx, client, rule, task.SourceBucket, remoteKey)
	}
}

func (s *Server) replicateDeleteObject(ctx context.Context, client *s3.Client, rule metadata.ReplicationRule, key string) (int64, error) {
	vis := s.sourceBucketVisibility(rule.SourceBucket)
	if err := s.ensureRemoteBucket(ctx, client, rule.DestBucket, vis); err != nil {
		return 0, err
	}
	_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(rule.DestBucket),
		Key:    aws.String(key),
	})
	if err != nil && !isS3NotFound(err) {
		return 0, err
	}
	return 0, nil
}

func (s *Server) replicatePutObject(ctx context.Context, client *s3.Client, rule metadata.ReplicationRule, srcBucket, key string) (int64, error) {
	srcKey := s.storageKeyForLogicalBucket(srcBucket)
	obj, err := s.meta.GetObject(srcKey, key)
	if err != nil {
		if isLocalNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	if obj.IsDeleteMarker || obj.Size == 0 {
		return 0, nil
	}
	rc, rec, err := s.svc.GetObject(ctx, srcBucket, key, "")
	if err != nil {
		if isLocalNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	body, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return 0, err
	}
	if err := s.ensureRemoteBucket(ctx, client, rule.DestBucket, s.sourceBucketVisibility(srcBucket)); err != nil {
		return 0, err
	}
	ct := rec.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(rule.DestBucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(ct),
	})
	if err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

func (s *Server) gatewayS3Client(conn metadata.GatewayConnection) (*s3.Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(conn.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			conn.AccessKey, conn.SecretKey, "",
		)),
	}
	if !conn.TLSVerify {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user-controlled gateway TLS
		opts = append(opts, awsconfig.WithHTTPClient(&http.Client{Transport: tr}))
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(conn.Endpoint)
		o.UsePathStyle = conn.PathStyle
	})
	return client, nil
}
