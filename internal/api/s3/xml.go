package s3

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
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
	XMLName     xml.Name     `xml:"ListBucketResult"`
	Xmlns       string       `xml:"xmlns,attr"`
	Name        string       `xml:"Name"`
	Prefix      string       `xml:"Prefix"`
	Marker      string       `xml:"Marker,omitempty"`
	NextMarker  string       `xml:"NextMarker,omitempty"`
	MaxKeys     int          `xml:"MaxKeys"`
	IsTruncated bool         `xml:"IsTruncated"`
	Contents    []ObjectInfo `xml:"Contents"`
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
	XMLName             xml.Name       `xml:"ListVersionsResult"`
	Xmlns               string         `xml:"xmlns,attr"`
	Name                string         `xml:"Name"`
	Prefix              string         `xml:"Prefix"`
	KeyMarker           string         `xml:"KeyMarker,omitempty"`
	VersionIDMarker     string         `xml:"VersionIdMarker,omitempty"`
	NextKeyMarker       string         `xml:"NextKeyMarker,omitempty"`
	NextVersionIDMarker string         `xml:"NextVersionIdMarker,omitempty"`
	KeyCount            int            `xml:"KeyCount"`
	MaxKeys             int            `xml:"MaxKeys"`
	IsTruncated         bool           `xml:"IsTruncated"`
	Items               []orderedEntry `xml:"-"`
}

// orderedEntry keeps a version and a delete marker in list order.
type orderedEntry struct {
	DeleteMarker bool
	Entry        VersionEntry
}

// VersionEntry is one object version or delete marker in ListObjectVersions.
type VersionEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag,omitempty"`
	Size         int64  `xml:"Size,omitempty"`
	StorageClass string `xml:"StorageClass,omitempty"`
	VersionID    string `xml:"VersionId,omitempty"`
	IsLatest     bool   `xml:"IsLatest"`
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
	Filter     *LifecycleFilter     `xml:"Filter,omitempty"`
	Status     string               `xml:"Status"`
	Expiration *LifecycleExpiration `xml:"Expiration,omitempty"`
}

type LifecycleFilter struct {
	Prefix string `xml:"Prefix,omitempty"`
}

type LifecycleExpiration struct {
	Days int `xml:"Days"`
}

type ListMultipartUploadsResult struct {
	XMLName            xml.Name               `xml:"ListMultipartUploadsResult"`
	Xmlns              string                 `xml:"xmlns,attr"`
	Bucket             string                 `xml:"Bucket"`
	Prefix             string                 `xml:"Prefix"`
	KeyMarker          string                 `xml:"KeyMarker"`
	UploadIDMarker     string                 `xml:"UploadIdMarker"`
	NextKeyMarker      string                 `xml:"NextKeyMarker,omitempty"`
	NextUploadIDMarker string                 `xml:"NextUploadIdMarker,omitempty"`
	MaxUploads         int                    `xml:"MaxUploads"`
	IsTruncated        bool                   `xml:"IsTruncated"`
	Upload             []MultipartUploadEntry `xml:"Upload,omitempty"`
}

type MultipartUploadEntry struct {
	Key          string `xml:"Key"`
	UploadID     string `xml:"UploadId"`
	StorageClass string `xml:"StorageClass"`
	Initiated    string `xml:"Initiated"`
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

// ParseLifecycleXML keeps prefix expiration (legacy Prefix, Filter/Prefix, or
// Filter/And/Prefix) and rejects every other action. An unsupported rule is
// not stored as a wider or zero-day rule.
func ParseLifecycleXML(data []byte) ([]metadata.LifecycleRule, error) {
	return parseLifecycleXML(data)
}

func parseLifecycleXML(data []byte) ([]metadata.LifecycleRule, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var rules []metadata.LifecycleRule
	var cur *lcRule
	var stack []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("malformed lifecycle xml")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			stack = append(stack, local)
			if local == "Rule" {
				cur = &lcRule{}
			}
			if cur == nil {
				continue
			}
			switch local {
			case "Transition", "NoncurrentVersionExpiration", "NoncurrentVersionTransition", "AbortIncompleteMultipartUpload", "ExpiredObjectDeleteMarker", "Tag", "ObjectSizeGreaterThan", "ObjectSizeLessThan":
				return nil, fmt.Errorf("unsupported lifecycle rule")
			}
			if local == "Date" && len(stack) >= 2 && stack[len(stack)-2] == "Expiration" {
				return nil, fmt.Errorf("unsupported lifecycle rule")
			}
		case xml.EndElement:
			if t.Name.Local == "Rule" && cur != nil {
				rule, err := cur.done()
				if err != nil {
					return nil, err
				}
				rules = append(rules, rule)
				cur = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if cur == nil {
				continue
			}
			text := strings.TrimSpace(string(t))
			if text == "" || len(stack) == 0 {
				continue
			}
			path := strings.Join(stack, "/")
			switch {
			case strings.HasSuffix(path, "/Rule/ID"):
				cur.id = text
			case strings.HasSuffix(path, "/Rule/Status"):
				cur.status = text
			case strings.HasSuffix(path, "/Rule/Prefix"):
				cur.legacy = text
				cur.hasLegacy = true
			case strings.HasSuffix(path, "/Filter/Prefix"), strings.HasSuffix(path, "/And/Prefix"):
				cur.filter = text
				cur.hasFilter = true
			case strings.HasSuffix(path, "/Expiration/Days"):
				n, err := strconv.Atoi(text)
				if err != nil {
					return nil, fmt.Errorf("malformed lifecycle xml")
				}
				cur.days = n
				cur.hasDays = true
			}
		}
	}
	return rules, nil
}

type lcRule struct {
	id, status, legacy, filter string
	days                       int
	hasLegacy, hasFilter       bool
	hasDays                    bool
}

func (c *lcRule) done() (metadata.LifecycleRule, error) {
	if c.hasLegacy && c.hasFilter {
		return metadata.LifecycleRule{}, fmt.Errorf("unsupported lifecycle rule")
	}
	if !c.hasDays || c.days < 1 {
		return metadata.LifecycleRule{}, fmt.Errorf("unsupported lifecycle rule")
	}
	prefix := c.filter
	if c.hasLegacy {
		prefix = c.legacy
	}
	id := c.id
	if id == "" {
		id = newLifecycleRuleID()
	}
	return metadata.LifecycleRule{
		ID:             id,
		Name:           id,
		Prefix:         prefix,
		Enabled:        strings.EqualFold(c.status, "Enabled"),
		Action:         metadata.LifecycleExpire,
		ExpirationDays: c.days,
	}, nil
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
			Status:     status,
			Expiration: &LifecycleExpiration{Days: r.ExpirationDays},
		}
		if r.Prefix != "" {
			xr.Filter = &LifecycleFilter{Prefix: r.Prefix}
		}
		cfg.Rules = append(cfg.Rules, xr)
	}
	return cfg
}

// MarshalXML writes versions and delete markers in list order.
func (r ListObjectVersionsResult) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "ListVersionsResult"}
	start.Attr = []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: xmlNS}}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	write := func(name, value string) error {
		if value == "" {
			return nil
		}
		return e.EncodeElement(value, xml.StartElement{Name: xml.Name{Local: name}})
	}
	if err := write("Name", r.Name); err != nil {
		return err
	}
	if err := write("Prefix", r.Prefix); err != nil {
		return err
	}
	if err := write("KeyMarker", r.KeyMarker); err != nil {
		return err
	}
	if err := write("VersionIdMarker", r.VersionIDMarker); err != nil {
		return err
	}
	if err := write("NextKeyMarker", r.NextKeyMarker); err != nil {
		return err
	}
	if err := write("NextVersionIdMarker", r.NextVersionIDMarker); err != nil {
		return err
	}
	if err := e.EncodeElement(r.MaxKeys, xml.StartElement{Name: xml.Name{Local: "MaxKeys"}}); err != nil {
		return err
	}
	if err := e.EncodeElement(r.IsTruncated, xml.StartElement{Name: xml.Name{Local: "IsTruncated"}}); err != nil {
		return err
	}
	if err := e.EncodeElement(r.KeyCount, xml.StartElement{Name: xml.Name{Local: "KeyCount"}}); err != nil {
		return err
	}
	for _, item := range r.Items {
		name := "Version"
		if item.DeleteMarker {
			name = "DeleteMarker"
		}
		if err := e.EncodeElement(item.Entry, xml.StartElement{Name: xml.Name{Local: name}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
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
	Id     string              `xml:"Id,omitempty"`
	Queue  string              `xml:"Queue,omitempty"`
	Filter *NotificationFilter `xml:"Filter,omitempty"`
	Events []string            `xml:"Event"`
}

type TopicConfiguration struct {
	Id     string              `xml:"Id,omitempty"`
	Topic  string              `xml:"Topic,omitempty"`
	Filter *NotificationFilter `xml:"Filter,omitempty"`
	Events []string            `xml:"Event"`
}

// NotificationFilter is the S3 key filter. Prefix and suffix are optional.
// A missing filter matches every key.
type NotificationFilter struct {
	S3Key *NotificationKeyFilter `xml:"S3Key,omitempty"`
	Key   *NotificationKeyFilter `xml:"Key,omitempty"`
}

type NotificationKeyFilter struct {
	Rules []NotificationFilterRule `xml:"FilterRule,omitempty"`
}

type NotificationFilterRule struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
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
