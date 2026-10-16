package migrate

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/policy"
)

// ObjectVersion is one source version or delete marker.
type ObjectVersion struct {
	Bucket       string
	Key          string
	VersionID    string
	ETag         string
	Size         int64
	LastModified time.Time
	IsLatest     bool
	DeleteMarker bool
	// Seq is document order from a newest-first listing. Higher Seq is older.
	// Zero means the caller did not record order; LastModified is used instead.
	Seq int
}

func versionIdentity(v ObjectVersion) string {
	if v.VersionID != "" {
		return v.VersionID
	}
	return "null"
}

// OrderVersions copies the oldest version of each key first.
func OrderVersions(in []ObjectVersion) []ObjectVersion {
	out := append([]ObjectVersion(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Bucket != out[j].Bucket {
			return out[i].Bucket < out[j].Bucket
		}
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		// A newest-first listing numbers the oldest entry with the higher Seq,
		// so that entry is written first and a delete marker stays between versions.
		if out[i].Seq != 0 && out[j].Seq != 0 && out[i].Seq != out[j].Seq {
			return out[i].Seq > out[j].Seq
		}
		if !out[i].LastModified.Equal(out[j].LastModified) {
			return out[i].LastModified.Before(out[j].LastModified)
		}
		return versionIdentity(out[i]) < versionIdentity(out[j])
	})
	return out
}

// PortPolicy keeps statements DataSafe can evaluate and reports the rest.
func PortPolicy(raw string) (string, []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if err := policy.Validate(raw); err == nil {
		return raw, nil
	}
	var doc policy.Document
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return "", []string{"bucket policy is not JSON and was not copied"}
	}
	var warnings []string
	kept := doc.Statement[:0]
	for _, st := range doc.Statement {
		var bad []string
		for op, keys := range st.Condition {
			switch strings.ToLower(op) {
			case "ipaddress", "notipaddress":
				for key := range keys {
					if !strings.EqualFold(key, "aws:SourceIp") {
						bad = append(bad, op+" "+key)
					}
				}
			case "bool":
				for key := range keys {
					if !strings.EqualFold(key, "aws:SecureTransport") {
						bad = append(bad, op+" "+key)
					}
				}
			default:
				named := false
				for key := range keys {
					bad = append(bad, op+" "+key)
					named = true
				}
				if !named {
					bad = append(bad, op)
				}
			}
		}
		if len(bad) > 0 {
			sid := st.Sid
			if sid == "" {
				sid = strings.Join(st.Action, ",")
			}
			warnings = append(warnings, "policy statement "+sid+" not copied: "+strings.Join(bad, ", "))
			continue
		}
		kept = append(kept, st)
	}
	doc.Statement = kept
	if len(doc.Statement) == 0 {
		warnings = append(warnings, "bucket policy had no supported statements and was not copied")
		return "", warnings
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", append(warnings, "bucket policy could not be encoded")
	}
	if err := policy.Validate(string(out)); err != nil {
		return "", append(warnings, "bucket policy still unsupported after dropping statements: "+err.Error())
	}
	return string(out), warnings
}

// LifecycleUnsupported is a non-nil error when the document is not a prefix
// expiration this server can store. The copier records a failure and does not PUT it.
func LifecycleUnsupported(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	_, err := s3.ParseLifecycleXML([]byte(raw))
	return err
}

// S3MultipartETag is the S3 multipart checksum of body split at partSize.
// Several parts use hex(md5(md5(part)…)) + "-" + count. One part is the plain MD5.
func S3MultipartETag(body []byte, partSize int64) string {
	if partSize < 1 {
		partSize = defaultPartSize
	}
	if len(body) == 0 {
		sum := md5.Sum(nil)
		return hex.EncodeToString(sum[:])
	}
	var parts [][]byte
	for off := 0; off < len(body); {
		n := int(partSize)
		if off+n > len(body) {
			n = len(body) - off
		}
		sum := md5.Sum(body[off : off+n])
		parts = append(parts, append([]byte(nil), sum[:]...))
		off += n
	}
	if len(parts) == 1 {
		return hex.EncodeToString(parts[0])
	}
	h := md5.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil)) + "-" + strconv.Itoa(len(parts))
}

// ETagsAgree reports whether the target etag matches the source, including a
// multipart source etag recomputed at partSize.
func ETagsAgree(source, target string, body []byte, partSize int64) bool {
	source = trimETag(source)
	target = trimETag(target)
	if source == "" || source == target {
		return true
	}
	return strings.Contains(source, "-") && S3MultipartETag(body, partSize) == source
}
