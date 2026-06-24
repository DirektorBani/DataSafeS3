package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	q := r.URL.Query().Get("q")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	owner := s.bucketOwnerFilter(info)
	includeUsers := auth.IsAdmin(info.Role)
	results, total, err := s.meta.Search(q, owner, includeUsers, offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

func (s *Server) handleListFavorites(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	items, err := s.meta.ListFavorites(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"favorites": items})
}

func (s *Server) handleCreateFavorite(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Type   string `json:"type"`
		Bucket string `json:"bucket"`
		Prefix string `json:"prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Bucket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bucket required"})
		return
	}
	if req.Type == "" {
		if req.Prefix != "" {
			req.Type = "folder"
		} else {
			req.Type = "bucket"
		}
	}
	if !s.canAccessBucket(info, req.Bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.FindFavorite(info.UserID, req.Type, req.Bucket, req.Prefix); err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "already favorited"})
		return
	}
	rec := metadata.FavoriteRecord{
		ID: randomID(), UserID: info.UserID, Type: req.Type,
		Bucket: req.Bucket, Prefix: req.Prefix, CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutFavorite(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"favorite": rec})
}

func (s *Server) handleDeleteFavorite(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	id := r.PathValue("id")
	if _, err := s.meta.GetFavorite(info.UserID, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err := s.meta.DeleteFavorite(info.UserID, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetBucketTags(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": rec.Tags})
}

func (s *Server) handlePutBucketTags(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Tags == nil {
		req.Tags = map[string]string{}
	}
	if err := s.meta.SetBucketTags(rec.EffectiveStorageKey(), req.Tags); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionBucketUpdated, "bucket", bucket)
	writeJSON(w, http.StatusOK, map[string]any{"tags": req.Tags})
}

func (s *Server) handleGetObjectTags(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	rec, err := s.meta.GetObjectVersion(sk, key, r.URL.Query().Get("versionId"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": rec.Tags})
}

func (s *Server) handlePutObjectTags(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Tags == nil {
		req.Tags = map[string]string{}
	}
	if err := s.meta.SetObjectTags(sk, key, r.URL.Query().Get("versionId"), req.Tags); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+key)
	writeJSON(w, http.StatusOK, map[string]any{"tags": req.Tags})
}

func (s *Server) handlePutObjectMeta(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Metadata    map[string]string `json:"metadata"`
		ContentType string            `json:"content_type"`
		Tags        map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	versionID := r.URL.Query().Get("versionId")
	if err := s.meta.UpdateObjectMeta(sk, key, versionID, req.Metadata, req.ContentType); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	if req.Tags != nil {
		_ = s.meta.SetObjectTags(sk, key, versionID, req.Tags)
	}
	rec, _ := s.meta.GetObjectVersion(sk, key, versionID)
	s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+key)
	writeJSON(w, http.StatusOK, map[string]any{"object": rec})
}

func (s *Server) handleInitiateMultipart(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Key         string `json:"key"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	uploadID, err := s.svc.CreateMultipartUpload(r.Context(), sk, req.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"upload_id": uploadID, "bucket": bucket, "key": req.Key,
	})
}

func (s *Server) handleUploadMultipartPart(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	uploadID := r.PathValue("uploadId")
	partStr := r.PathValue("partNumber")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	mp, err := s.meta.GetMultipart(uploadID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "upload not found"})
		return
	}
	partNum, err := strconv.Atoi(partStr)
	if err != nil || partNum <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid part number"})
		return
	}
	etag, err := s.svc.UploadPart(r.Context(), mp.Bucket, mp.Key, uploadID, partNum, r.Body, r.ContentLength)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"etag": etag, "part_number": partNum})
}

func (s *Server) handleCompleteMultipart(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	uploadID := r.PathValue("uploadId")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	mp, err := s.meta.GetMultipart(uploadID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "upload not found"})
		return
	}
	var req struct {
		Parts []struct {
			PartNumber int    `json:"part_number"`
			ETag       string `json:"etag"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Parts) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "parts required"})
		return
	}
	var parts []storage.PartInfo
	for _, p := range req.Parts {
		parts = append(parts, storage.PartInfo{PartNumber: p.PartNumber, ETag: strings.Trim(p.ETag, `"`)})
	}
	rec, err := s.svc.CompleteMultipartUpload(r.Context(), mp.Bucket, mp.Key, uploadID, parts)
	if err != nil {
		if err == metadata.ErrQuotaExceeded {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "quota exceeded"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+mp.Key)
	s.emitEvent(metadata.EventObjectCreated, map[string]any{"bucket": bucket, "key": mp.Key, "size": rec.Size})
	s.emitEvent(metadata.EventMultipartCompleted, map[string]any{"bucket": bucket, "key": mp.Key, "upload_id": uploadID})
	writeJSON(w, http.StatusOK, map[string]any{"object": rec})
}

func (s *Server) handleAbortMultipart(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	uploadID := r.PathValue("uploadId")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	mp, err := s.meta.GetMultipart(uploadID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "upload not found"})
		return
	}
	if err := s.svc.AbortMultipartUpload(r.Context(), mp.Bucket, mp.Key, uploadID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.meta.GetWebhook(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	items, err := s.meta.ListWebhookDeliveries(id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": items})
}

func (s *Server) handleRetryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	webhookID := r.PathValue("id")
	deliveryID := r.PathValue("deliveryId")
	hook, err := s.meta.GetWebhook(webhookID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "webhook not found"})
		return
	}
	delivery, err := s.meta.GetWebhookDelivery(deliveryID)
	if err != nil || delivery.WebhookID != webhookID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "delivery not found"})
		return
	}
	go s.deliverWebhook(hook, delivery.Payload, &delivery)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "delivery_id": deliveryID})
}

func parseMaxKeys(r *http.Request, defaultVal, maxVal int) int {
	v, err := strconv.Atoi(r.URL.Query().Get("max_keys"))
	if err != nil || v <= 0 {
		return defaultVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
