package s3

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

func (h *Handler) getBucketObjectLock(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetBucketObjectLockConfiguration") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	cfg, err := h.Svc.GetObjectLockConfiguration(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, cfg)
}

func (h *Handler) putBucketObjectLock(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketObjectLockConfiguration") {
		return
	}
	if !h.Svc.CallerIsBucketAdmin(creds.AccessKey, bucket) {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "bucket admin required")
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	var cfg ObjectLockConfiguration
	if err := xml.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.PutObjectLockConfiguration(sk, cfg); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getObjectRetention(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObjectRetention") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	ret, err := h.Svc.GetObjectRetention(sk, key, versionID)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, ret)
}

func (h *Handler) putObjectRetention(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObjectRetention") {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	body, err := parseRetentionXML(raw)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	if strings.EqualFold(body.Mode, "COMPLIANCE") && !h.Svc.CallerIsBucketAdmin(creds.AccessKey, bucket) {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "bucket admin required")
		return
	}
	until, _ := time.Parse(time.RFC3339, body.RetainUntilDate)
	var existing *time.Time
	if preview, _, err := h.Svc.ResolveBucketKey(creds.AccessKey, bucket); err == nil {
		if obj, err := h.Svc.Meta.GetObject(preview, key); err == nil {
			existing = obj.RetentionUntil
		}
	}
	if complianceOrShorten(body.Mode, existing, until) && h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	if err := h.Svc.PutObjectRetention(sk, key, versionID, body); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getObjectLegalHold(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObjectLegalHold") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	hold, err := h.Svc.GetObjectLegalHold(sk, key, versionID)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, ObjectLockLegalHold{Status: hold})
}

func (h *Handler) putObjectLegalHold(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObjectLegalHold") {
		return
	}
	var body ObjectLockLegalHold
	if err := xml.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	hold := strings.EqualFold(body.Status, "ON")
	if err := h.Svc.PutObjectLegalHold(sk, key, versionID, hold); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getBucketNotification(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetBucketNotification") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	cfg, err := h.Svc.GetBucketNotificationConfiguration(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, cfg)
}

func (h *Handler) putBucketNotification(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketNotification") {
		return
	}
	var cfg NotificationConfiguration
	if err := xml.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.PutBucketNotificationConfiguration(sk, cfg); err != nil {
		if strings.Contains(err.Error(), "outbound url not allowed") {
			writeError(w, r, http.StatusBadRequest, "InvalidArgument", err.Error())
			return
		}
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteBucketNotification(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketNotification") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.DeleteBucketNotificationConfiguration(sk); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) GetObjectLockConfiguration(bucket string) (ObjectLockConfiguration, error) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return ObjectLockConfiguration{}, err
	}
	cfg := ObjectLockConfiguration{Xmlns: xmlNS}
	if rec.ObjectLock {
		cfg.ObjectLockEnabled = "Enabled"
		if rec.RetentionDays > 0 || rec.RetentionMode != "" {
			mode := rec.RetentionMode
			if mode == "" {
				mode = metadata.RetentionGovernance
			}
			cfg.Rule = &ObjectLockRule{
				DefaultRetention: &DefaultRetention{
					Mode: mode,
					Days: rec.RetentionDays,
				},
			}
		}
	}
	return cfg, nil
}

func (s *Service) PutObjectLockConfiguration(bucket string, cfg ObjectLockConfiguration) error {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return err
	}
	rec.ObjectLock = strings.EqualFold(cfg.ObjectLockEnabled, "Enabled")
	if cfg.Rule != nil && cfg.Rule.DefaultRetention != nil {
		dr := cfg.Rule.DefaultRetention
		if dr.Days > 0 {
			rec.RetentionDays = dr.Days
		} else if dr.Years > 0 {
			rec.RetentionDays = dr.Years * 365
		}
		if dr.Mode != "" {
			rec.RetentionMode = strings.ToUpper(dr.Mode)
		}
	}
	return s.Meta.UpdateBucket(rec)
}

func parseRetentionXML(data []byte) (ObjectLockRetention, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var ret ObjectLockRetention
	var stack []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ret, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || len(stack) == 0 {
				continue
			}
			switch stack[len(stack)-1] {
			case "Mode":
				ret.Mode = text
			case "RetainUntilDate":
				ret.RetainUntilDate = text
			}
		}
	}
	if ret.Mode == "" && ret.RetainUntilDate == "" {
		return ret, fmt.Errorf("empty retention")
	}
	return ret, nil
}

func (s *Service) GetObjectRetention(bucket, key, versionID string) (ObjectLockRetention, error) {
	bucket = s.normalizeBucketKey(bucket)
	obj, err := s.Meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return ObjectLockRetention{}, err
	}
	mode := strings.ToUpper(strings.TrimSpace(obj.RetentionMode))
	if mode == "" {
		mode = metadata.RetentionGovernance
		if brec, err := s.Meta.GetBucketByKey(bucket); err == nil && brec.RetentionMode != "" {
			mode = brec.RetentionMode
		}
	}
	ret := ObjectLockRetention{Mode: mode}
	if obj.RetentionUntil != nil {
		ret.RetainUntilDate = obj.RetentionUntil.UTC().Format(time.RFC3339)
	}
	return ret, nil
}

func (s *Service) PutObjectRetention(bucket, key, versionID string, body ObjectLockRetention) error {
	bucket = s.normalizeBucketKey(bucket)
	untilStr := body.RetainUntilDate
	if untilStr == "" {
		return metadata.ErrNotFound
	}
	until, err := time.Parse(time.RFC3339, untilStr)
	if err != nil {
		until, err = time.Parse("2006-01-02T15:04:05.000Z", untilStr)
		if err != nil {
			return err
		}
	}
	if err := s.Meta.SetObjectRetention(bucket, key, versionID, until, body.Mode); err != nil {
		return err
	}
	brec, err := s.Meta.GetBucketByKey(bucket)
	if err == nil {
		brec.ObjectLock = true
		if body.Mode != "" {
			brec.RetentionMode = strings.ToUpper(body.Mode)
		}
		_ = s.Meta.UpdateBucket(brec)
	}
	return nil
}

func (s *Service) GetObjectLegalHold(bucket, key, versionID string) (string, error) {
	bucket = s.normalizeBucketKey(bucket)
	obj, err := s.Meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return "", err
	}
	if obj.LegalHold {
		return "ON", nil
	}
	return "OFF", nil
}

func (s *Service) PutObjectLegalHold(bucket, key, versionID string, hold bool) error {
	bucket = s.normalizeBucketKey(bucket)
	return s.Meta.SetObjectLegalHold(bucket, key, versionID, hold)
}

func (s *Service) GetBucketNotificationConfiguration(bucket string) (NotificationConfiguration, error) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return NotificationConfiguration{}, err
	}
	cfg, ok := metadata.BucketNotificationFromTags(rec.Tags)
	if !ok {
		return NotificationConfiguration{Xmlns: xmlNS}, nil
	}
	out := NotificationConfiguration{Xmlns: xmlNS}
	if cfg.WebhookURL != "" {
		out.TopicConfiguration = []TopicConfiguration{{
			Id:     "webhook",
			Topic:  cfg.WebhookURL,
			Filter: notificationFilterXML(cfg.Prefix, cfg.Suffix),
			Events: cfg.Events,
		}}
	}
	return out, nil
}

func (s *Service) PutBucketNotificationConfiguration(bucket string, xmlCfg NotificationConfiguration) error {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return err
	}
	var webhookURL, prefix, suffix string
	var events []string
	for _, t := range xmlCfg.TopicConfiguration {
		if t.Topic != "" {
			if err := urlpolicy.ValidateOutboundURL(t.Topic, urlpolicy.DefaultOptions()); err != nil {
				return fmt.Errorf("%s", urlpolicy.OutboundURLError(err))
			}
			webhookURL = t.Topic
			prefix, suffix = notificationFilterParts(t.Filter)
			events = append(events, t.Events...)
		}
	}
	for _, q := range xmlCfg.QueueConfiguration {
		if q.Queue != "" {
			if err := urlpolicy.ValidateOutboundURL(q.Queue, urlpolicy.DefaultOptions()); err != nil {
				return fmt.Errorf("%s", urlpolicy.OutboundURLError(err))
			}
			webhookURL = q.Queue
			prefix, suffix = notificationFilterParts(q.Filter)
			events = append(events, q.Events...)
		}
	}
	rec.Tags = metadata.SetBucketNotificationTag(rec.Tags, metadata.BucketNotificationConfig{
		WebhookURL: webhookURL,
		Events:     events,
		Prefix:     prefix,
		Suffix:     suffix,
	})
	return s.Meta.UpdateBucket(rec)
}

func (s *Service) DeleteBucketNotificationConfiguration(bucket string) error {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return err
	}
	rec.Tags = metadata.SetBucketNotificationTag(rec.Tags, metadata.BucketNotificationConfig{})
	return s.Meta.UpdateBucket(rec)
}

func (s *Service) EmitBucketNotification(bucket, event, key string, size int64) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return
	}
	cfg, ok := metadata.BucketNotificationFromTags(rec.Tags)
	if !ok || cfg.WebhookURL == "" {
		return
	}
	kind := "put"
	if event == metadata.ReplEventDelete || event == "delete" || event == "ObjectRemoved" {
		kind = "delete"
	}
	if !metadata.BucketNotificationMatches(cfg, kind, key) {
		return
	}
	if s.OnBucketNotification != nil {
		s.OnBucketNotification(cfg.WebhookURL, event, bucket, key, size)
	}
}

func notificationFilterParts(f *NotificationFilter) (prefix, suffix string) {
	if f == nil {
		return "", ""
	}
	keys := []*NotificationKeyFilter{f.S3Key, f.Key}
	for _, k := range keys {
		if k == nil {
			continue
		}
		for _, rule := range k.Rules {
			switch strings.ToLower(strings.TrimSpace(rule.Name)) {
			case "prefix":
				prefix = rule.Value
			case "suffix":
				suffix = rule.Value
			}
		}
	}
	return prefix, suffix
}

func notificationFilterXML(prefix, suffix string) *NotificationFilter {
	if prefix == "" && suffix == "" {
		return nil
	}
	var rules []NotificationFilterRule
	if prefix != "" {
		rules = append(rules, NotificationFilterRule{Name: "prefix", Value: prefix})
	}
	if suffix != "" {
		rules = append(rules, NotificationFilterRule{Name: "suffix", Value: suffix})
	}
	return &NotificationFilter{S3Key: &NotificationKeyFilter{Rules: rules}}
}
