package s3

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

type ErrorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId,omitempty"`
}

type ListAllMyBucketsResult struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`
	Xmlns   string   `xml:"xmlns,attr"`
	Owner   Owner    `xml:"Owner"`
	Buckets Buckets  `xml:"Buckets"`
}

type Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type Buckets struct {
	Bucket []BucketEntry `xml:"Bucket"`
}

type BucketEntry struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type ListBucketResult struct {
	XMLName               xml.Name     `xml:"ListBucketResult"`
	Xmlns                 string       `xml:"xmlns,attr"`
	Name                  string       `xml:"Name"`
	Prefix                string       `xml:"Prefix"`
	KeyCount              int          `xml:"KeyCount"`
	MaxKeys               int          `xml:"MaxKeys"`
	IsTruncated           bool         `xml:"IsTruncated"`
	Contents              []ObjectInfo `xml:"Contents"`
	ContinuationToken     string       `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string       `xml:"NextContinuationToken,omitempty"`
}

type ListObjectsV2Result struct {
	XMLName               xml.Name     `xml:"ListBucketResult"`
	Xmlns                 string       `xml:"xmlns,attr"`
	Name                  string       `xml:"Name"`
	Prefix                string       `xml:"Prefix"`
	KeyCount              int          `xml:"KeyCount"`
	MaxKeys               int          `xml:"MaxKeys"`
	IsTruncated           bool         `xml:"IsTruncated"`
	Contents              []ObjectInfo `xml:"Contents"`
	ContinuationToken     string       `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string       `xml:"NextContinuationToken,omitempty"`
}

type ObjectInfo struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
	VersionID    string `xml:"VersionId,omitempty"`
}

type ListObjectVersionsResult struct {
	XMLName     xml.Name     `xml:"ListVersionsResult"`
	Xmlns       string       `xml:"xmlns,attr"`
	Name        string       `xml:"Name"`
	Prefix      string       `xml:"Prefix"`
	KeyCount    int          `xml:"KeyCount"`
	MaxKeys     int          `xml:"MaxKeys"`
	IsTruncated bool         `xml:"IsTruncated"`
	Versions    []ObjectInfo `xml:"Version"`
}

type VersioningConfiguration struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	Xmlns   string   `xml:"xmlns,attr"`
	Status  string   `xml:"Status"`
}

type LifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Xmlns   string          `xml:"xmlns,attr"`
	Rules   []LifecycleRule `xml:"Rule"`
}

type LifecycleRule struct {
	ID         string               `xml:"ID"`
	Prefix     string               `xml:"Prefix,omitempty"`
	Status     string               `xml:"Status"`
	Expiration *LifecycleExpiration `xml:"Expiration,omitempty"`
}

type LifecycleExpiration struct {
	Days int `xml:"Days"`
}

type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Xmlns    string   `xml:"xmlns,attr"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Xmlns    string   `xml:"xmlns,attr"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type CompleteMultipartUpload struct {
	XMLName xml.Name `xml:"CompleteMultipartUpload"`
	Part    []Part   `xml:"Part"`
}

type Part struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

const xmlNS = "http://s3.amazonaws.com/doc/2006-03-01/"

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func xmlHeader() string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
}

func marshalXML(v any) ([]byte, error) {
	data, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xmlHeader()), data...), nil
}

func copySourceBucketKey(src string) (bucket, key string, ok bool) {
	if len(src) > 0 && src[0] == '/' {
		src = src[1:]
	}
	i := 0
	for i < len(src) && src[i] != '/' {
		i++
	}
	if i >= len(src) {
		return src, "", true
	}
	return src[:i], src[i+1:], true
}

func location(bucket, key string) string {
	return fmt.Sprintf("/%s/%s", bucket, key)
}

func lifecycleRulesFromXML(cfg LifecycleConfiguration) []metadata.LifecycleRule {
	var rules []metadata.LifecycleRule
	for _, r := range cfg.Rules {
		rule := metadata.LifecycleRule{
			ID:      r.ID,
			Name:    r.ID,
			Prefix:  r.Prefix,
			Enabled: strings.EqualFold(r.Status, "Enabled"),
			Action:  metadata.LifecycleExpire,
		}
		if r.Expiration != nil {
			rule.ExpirationDays = r.Expiration.Days
		}
		if rule.ID == "" {
			rule.ID = newLifecycleRuleID()
		}
		rules = append(rules, rule)
	}
	return rules
}

func lifecycleRulesToXML(rules []metadata.LifecycleRule) LifecycleConfiguration {
	cfg := LifecycleConfiguration{Xmlns: xmlNS}
	for _, r := range rules {
		id := r.ID
		if id == "" {
			id = r.Name
		}
		status := "Disabled"
		if r.Enabled {
			status = "Enabled"
		}
		xr := LifecycleRule{
			ID:         id,
			Prefix:     r.Prefix,
			Status:     status,
			Expiration: &LifecycleExpiration{Days: r.ExpirationDays},
		}
		cfg.Rules = append(cfg.Rules, xr)
	}
	return cfg
}

func newLifecycleRuleID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  TagSet   `xml:"TagSet"`
}

type TagSet struct {
	Tags []TagEntry `xml:"Tag"`
}

type TagEntry struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

type ObjectLockConfiguration struct {
	XMLName           xml.Name        `xml:"ObjectLockConfiguration"`
	Xmlns             string          `xml:"xmlns,attr"`
	ObjectLockEnabled string          `xml:"ObjectLockEnabled,omitempty"`
	Rule              *ObjectLockRule `xml:"Rule,omitempty"`
}

type ObjectLockRule struct {
	DefaultRetention *DefaultRetention `xml:"DefaultRetention,omitempty"`
}

type DefaultRetention struct {
	Mode  string `xml:"Mode,omitempty"`
	Days  int    `xml:"Days,omitempty"`
	Years int    `xml:"Years,omitempty"`
}

type ObjectLockRetention struct {
	XMLName         xml.Name `xml:"Retention"`
	Mode            string   `xml:"Mode"`
	RetainUntilDate string   `xml:"RetainUntilDate,omitempty"`
}

type ObjectLockLegalHold struct {
	XMLName xml.Name `xml:"LegalHold"`
	Status  string   `xml:"Status"`
}

type NotificationConfiguration struct {
	XMLName                    xml.Name                     `xml:"NotificationConfiguration"`
	Xmlns                      string                       `xml:"xmlns,attr"`
	QueueConfiguration         []QueueConfiguration         `xml:"QueueConfiguration,omitempty"`
	TopicConfiguration         []TopicConfiguration         `xml:"TopicConfiguration,omitempty"`
	CloudFunctionConfiguration []CloudFunctionConfiguration `xml:"CloudFunctionConfiguration,omitempty"`
}

type QueueConfiguration struct {
	Id     string   `xml:"Id,omitempty"`
	Queue  string   `xml:"Queue,omitempty"`
	Events []string `xml:"Event"`
}

type TopicConfiguration struct {
	Id     string   `xml:"Id,omitempty"`
	Topic  string   `xml:"Topic,omitempty"`
	Events []string `xml:"Event"`
}

type CloudFunctionConfiguration struct {
	Id            string   `xml:"Id,omitempty"`
	CloudFunction string   `xml:"CloudFunction,omitempty"`
	Events        []string `xml:"Event"`
}

func tagsToMap(tags []TagEntry) map[string]string {
	out := map[string]string{}
	for _, t := range tags {
		if t.Key != "" {
			out[t.Key] = t.Value
		}
	}
	return out
}

func mapToTagEntries(tags map[string]string) []TagEntry {
	var out []TagEntry
	for k, v := range tags {
		out = append(out, TagEntry{Key: k, Value: v})
	}
	return out
}
