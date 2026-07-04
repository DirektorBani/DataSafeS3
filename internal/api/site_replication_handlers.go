package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func siteReplEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_SITE_REPLICATION_ENABLED")))
	return v == "true" || v == "1"
}

func (s *Server) enqueueSiteReplication(event, bucket, key string) {
	if bucket == metadata.TrashBucketName || key == "" {
		return
	}
	if !siteReplEnabled() && !trustedClusterReplEnabled() {
		return
	}
	logicalBucket := bucket
	if rec, err := s.meta.GetBucketByKey(bucket); err == nil {
		logicalBucket = rec.Name
	} else if rec, err := s.meta.GetBucket(bucket); err == nil {
		logicalBucket = rec.Name
	}
	rules, err := s.meta.ListSiteReplicationRulesForBucket(logicalBucket)
	if err != nil || len(rules) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, rule := range rules {
		if rule.TrustedClusterID != "" {
			if !trustedClusterReplEnabled() {
				continue
			}
			if !s.trustedClusterHealthy(rule.TrustedClusterID) {
				continue
			}
		} else if !siteReplEnabled() {
			continue
		}
		task := metadata.SiteReplicationTask{
			ID:           randomID(),
			RuleID:       rule.ID,
			Event:        event,
			SourceBucket: logicalBucket,
			Key:          key,
			Status:       metadata.SiteReplTaskPending,
			CreatedAt:    now,
			NextAttempt:  now,
		}
		_ = s.meta.PutSiteReplicationTask(task)
	}
}

func (s *Server) runSiteReplicationWorker(ctx context.Context) {
	if !siteReplWorkerEnabled() {
		return
	}
	interval := 2 * time.Second
	if v := os.Getenv("STORAGE_SITE_REPL_WORKER_INTERVAL"); v != "" {
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
			s.processSiteReplicationOnce(ctx)
		}
	}
}

func (s *Server) processSiteReplicationOnce(ctx context.Context) {
	tasks, err := s.meta.ListDueSiteReplicationTasks(20, time.Now().UTC())
	if err != nil {
		return
	}
	for _, task := range tasks {
		if err := s.processSiteReplicationTask(ctx, task); err != nil {
			task.Attempts++
			task.Error = err.Error()
			task.Status = metadata.SiteReplTaskFailed
			if task.Attempts < 5 {
				task.Status = metadata.SiteReplTaskPending
				task.NextAttempt = time.Now().UTC().Add(time.Duration(task.Attempts*task.Attempts) * time.Second)
			}
			_ = s.meta.PutSiteReplicationTask(task)
			continue
		}
		now := time.Now().UTC()
		task.Status = metadata.SiteReplTaskDone
		task.ProcessedAt = &now
		_ = s.meta.PutSiteReplicationTask(task)
	}
	st, _ := s.meta.SiteReplicationStatus()
	observability.SetSiteReplicationMetrics(st.PendingCount, st.LagSeconds)
}

func (s *Server) processSiteReplicationTask(ctx context.Context, task metadata.SiteReplicationTask) error {
	rules, err := s.meta.ListSiteReplicationRules()
	if err != nil {
		return err
	}
	var rule metadata.SiteReplicationRule
	for _, r := range rules {
		if r.ID == task.RuleID {
			rule = r
			break
		}
	}
	if rule.ID == "" || !rule.Enabled {
		return metadata.ErrNotFound
	}
	client, _, err := s.siteReplS3ClientForRule(ctx, rule)
	if err != nil {
		return err
	}
	sk := s.storageKeyForLogicalBucket(task.SourceBucket)
	switch task.Event {
	case metadata.SiteReplEventDelete:
		_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(rule.DestBucket), Key: aws.String(task.Key)})
		return err
	default:
		rc, _, err := s.svc.GetObject(ctx, sk, task.Key, "")
		if err != nil {
			return err
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		task.Bytes = int64(len(body))
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(rule.DestBucket),
			Key:    aws.String(task.Key),
			Body:   bytes.NewReader(body),
		})
		return err
	}
}

func siteReplS3Client(peer metadata.SiteReplicationPeer) (*s3.Client, error) {
	endpoint := strings.TrimSuffix(peer.Endpoint, "/")
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(peer.AccessKey, peer.SecretKey, "")),
		awsconfig.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}

func (s *Server) handleListSiteReplicationPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := s.meta.ListSiteReplicationPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for i := range peers {
		peers[i].SecretKey = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}

func (s *Server) handleCreateSiteReplicationPeer(w http.ResponseWriter, r *http.Request) {
	var req metadata.SiteReplicationPeer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.ID == "" {
		req.ID = randomID()
	}
	req.CreatedAt = time.Now().UTC()
	if err := s.meta.PutSiteReplicationPeer(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	req.SecretKey = ""
	writeJSON(w, http.StatusCreated, map[string]any{"peer": req})
}

func (s *Server) handleDeleteSiteReplicationPeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteSiteReplicationPeer(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) handleListSiteReplicationRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.meta.ListSiteReplicationRules()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (s *Server) handleCreateSiteReplicationRule(w http.ResponseWriter, r *http.Request) {
	var req metadata.SiteReplicationRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.ID == "" {
		req.ID = randomID()
	}
	if req.Direction == "" {
		req.Direction = "one-way"
	}
	if req.PeerID == "" && req.TrustedClusterID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "peer_id or trusted_cluster_id required"})
		return
	}
	if req.PeerID != "" && req.TrustedClusterID != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "peer_id and trusted_cluster_id are mutually exclusive"})
		return
	}
	req.CreatedAt = time.Now().UTC()
	if err := s.meta.PutSiteReplicationRule(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rule": req})
}

func (s *Server) handleDeleteSiteReplicationRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteSiteReplicationRule(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) handleTriggerSiteReplicationSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rules, _ := s.meta.ListSiteReplicationRules()
	for _, rule := range rules {
		if rule.ID != id {
			continue
		}
		objs, err := s.meta.ListObjects(s.storageKeyForLogicalBucket(rule.SourceBucket), "", 0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		now := time.Now().UTC()
		for _, obj := range objs {
			if obj.IsDeleteMarker {
				continue
			}
			_ = s.meta.PutSiteReplicationTask(metadata.SiteReplicationTask{
				ID: randomID(), RuleID: rule.ID, Event: metadata.SiteReplEventPut,
				SourceBucket: rule.SourceBucket, Key: obj.Key, Status: metadata.SiteReplTaskPending,
				CreatedAt: now, NextAttempt: now,
			})
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"sync": "queued", "rule_id": id})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
}

func (s *Server) handleSiteReplicationStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.meta.SiteReplicationStatus()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}
