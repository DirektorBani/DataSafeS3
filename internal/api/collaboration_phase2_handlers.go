package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func collaborationNotifyID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type grantNotifyKey struct {
	userID string
	prefix string
}

func grantNotifyKeys(grants []metadata.BucketAccessGrant, prefixGrants []metadata.BucketPrefixAccessGrant) map[grantNotifyKey]struct{} {
	out := map[grantNotifyKey]struct{}{}
	for _, g := range grants {
		if g.CanRead || g.CanWrite {
			out[grantNotifyKey{userID: g.UserID}] = struct{}{}
		}
	}
	for _, g := range prefixGrants {
		if g.CanRead || g.CanWrite {
			out[grantNotifyKey{userID: g.UserID, prefix: g.Prefix}] = struct{}{}
		}
	}
	return out
}

func grantNotifyKeysFromViews(grants []bucketAccessGrantView) map[grantNotifyKey]struct{} {
	out := map[grantNotifyKey]struct{}{}
	for _, g := range grants {
		if g.CanRead || g.CanWrite {
			out[grantNotifyKey{userID: g.UserID, prefix: g.Prefix}] = struct{}{}
		}
	}
	return out
}

func (s *Server) notifyBucketSharedDiff(
	actor auth.TokenInfo,
	bucket string,
	oldGrants []bucketAccessGrantView,
	oldPrefix []bucketAccessGrantView,
	grants []metadata.BucketAccessGrant,
	prefixGrants []metadata.BucketPrefixAccessGrant,
) {
	before := grantNotifyKeysFromViews(oldGrants)
	beforePrefix := grantNotifyKeysFromViews(oldPrefix)
	for k := range beforePrefix {
		before[k] = struct{}{}
	}

	seen := map[string]struct{}{}
	notify := func(userID, body, link string) {
		if userID == "" || userID == actor.UserID {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		seen[userID] = struct{}{}
		_ = s.meta.PutUserNotification(metadata.UserNotificationRecord{
			ID:        collaborationNotifyID(),
			UserID:    userID,
			Kind:      "bucket_shared",
			Title:     "Files shared with you",
			Body:      body,
			Link:      link,
			CreatedAt: time.Now().UTC(),
		})
	}

	link := "/buckets/" + bucket
	for _, g := range grants {
		if !(g.CanRead || g.CanWrite) {
			continue
		}
		key := grantNotifyKey{userID: g.UserID}
		if _, was := before[key]; was {
			continue
		}
		mode := "read"
		if g.CanWrite {
			mode = "read and write"
		}
		notify(g.UserID, fmt.Sprintf("%s shared bucket %q with %s access", actor.Username, bucket, mode), link)
	}
	for _, g := range prefixGrants {
		if !(g.CanRead || g.CanWrite) {
			continue
		}
		key := grantNotifyKey{userID: g.UserID, prefix: g.Prefix}
		if _, was := before[key]; was {
			continue
		}
		mode := "read"
		if g.CanWrite {
			mode = "read and write"
		}
		notify(g.UserID, fmt.Sprintf("%s shared folder %q in bucket %q (%s)", actor.Username, g.Prefix, bucket, mode), link+"?tab=objects&prefix="+g.Prefix)
	}
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	items, err := s.meta.ListUserNotifications(info.UserID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	unread, _ := s.meta.CountUnreadNotifications(info.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	id := r.PathValue("id")
	if err := s.meta.MarkUserNotificationRead(info.UserID, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	if err := s.meta.MarkAllUserNotificationsRead(info.UserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRecent(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	items, err := s.meta.ListRecentItems(info.UserID, 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
