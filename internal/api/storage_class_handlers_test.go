package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestTransitionStorageClassHotToWarm(t *testing.T) {
	s := testServer(t)
	token, err := s.AdminToken()
	if err != nil {
		t.Fatal(err)
	}
	bucket := "sc-bucket"
	_ = s.Meta().PutBucket(metadata.BucketRecord{Name: bucket, Owner: "admin", StorageClass: metadata.StorageClassHot, CreatedAt: time.Now().UTC()})
	sk := bucket
	_ = s.Meta().PutObject(metadata.ObjectRecord{Bucket: sk, Key: "f.txt", Size: 1, StorageClass: metadata.StorageClassHot})

	body, _ := json.Marshal(map[string]string{"key": "f.txt", "storage_class": metadata.StorageClassWarm})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/"+bucket+"/objects/transition-storage-class", bytes.NewReader(body))
	req.SetPathValue("bucket", bucket)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	obj, err := s.Meta().GetObject(sk, "f.txt")
	if err != nil || obj.StorageClass != metadata.StorageClassWarm {
		t.Fatalf("expected warm got %+v err=%v", obj, err)
	}
}
