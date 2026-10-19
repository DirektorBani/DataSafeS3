package s3

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"hash/crc32"
	"hash/crc64"
	"io"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
)

const (
	deleteObjectsMaxKeys = 1000
	deleteObjectsMaxBody = 2 << 20
	s3XMLNS              = "http://s3.amazonaws.com/doc/2006-03-01/"
)

// crc64NVMEPoly matches the reflected AWS CRC64NVME polynomial used for
// chunked uploads. See internal/auth/chunked.go.
const crc64NVMEPoly = 0x9A6C9329AC4BC9B5

var errDeleteDigest = errors.New("bad digest")

type deleteObjectsRequest struct {
	XMLName xml.Name             `xml:"Delete"`
	Quiet   bool                 `xml:"Quiet"`
	Objects []deleteObjectsEntry `xml:"Object"`
}

type deleteObjectsEntry struct {
	Key       string `xml:"Key"`
	VersionID string `xml:"VersionId"`
}

type deleteObjectsResponse struct {
	XMLName xml.Name              `xml:"DeleteResult"`
	Xmlns   string                `xml:"xmlns,attr"`
	Deleted []deletedObjectResult `xml:"Deleted,omitempty"`
	Errors  []deleteObjectError   `xml:"Error,omitempty"`
}

type deletedObjectResult struct {
	Key                   string `xml:"Key"`
	VersionID             string `xml:"VersionId,omitempty"`
	DeleteMarker          *bool  `xml:"DeleteMarker,omitempty"`
	DeleteMarkerVersionID string `xml:"DeleteMarkerVersionId,omitempty"`
}

type deleteObjectError struct {
	Key       string `xml:"Key"`
	VersionID string `xml:"VersionId,omitempty"`
	Code      string `xml:"Code"`
	Message   string `xml:"Message"`
}

func (h *Handler) deleteObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("X-Amz-Algorithm") != "" {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "presigned DeleteObjects is not allowed")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	body, err := readDeleteBody(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "The XML you provided was not well-formed or did not validate against our published schema")
		return
	}
	if err := validateDeleteChecksums(r.Header, body); err != nil {
		if errors.Is(err, errDeleteDigest) {
			writeError(w, r, http.StatusBadRequest, "BadDigest", "The Content-MD5 or checksum you specified did not match what we received")
			return
		}
		writeError(w, r, http.StatusBadRequest, "InvalidRequest", "Missing required header for this request: Content-MD5 or x-amz-checksum-*")
		return
	}
	var req deleteObjectsRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "The XML you provided was not well-formed or did not validate against our published schema")
		return
	}
	if len(req.Objects) > deleteObjectsMaxKeys {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "The request contains more than 1000 keys")
		return
	}
	headerBypass := governanceHeader(r)
	resp := deleteObjectsResponse{Xmlns: s3XMLNS}
	ctx := WithObjectEvents(r.Context())
	for _, obj := range req.Objects {
		key := obj.Key
		versionID := obj.VersionID
		if key == "" {
			resp.Errors = append(resp.Errors, deleteObjectError{
				Key: key, VersionID: versionID,
				Code: "UserKeyMustBeSpecified", Message: "The request did not specify a key",
			})
			continue
		}
		if err := storage.ValidateObjectKey(key); err != nil {
			resp.Errors = append(resp.Errors, deleteObjectError{
				Key: key, VersionID: versionID,
				Code: "InvalidArgument", Message: err.Error(),
			})
			continue
		}
		action := "s3:DeleteObject"
		if versionID != "" {
			action = "s3:DeleteObjectVersion"
		}
		if !h.Svc.AuthorizeRequest(bucket, key, action, creds.AccessKey, requestSourceIP(r), r.TLS != nil) {
			resp.Errors = append(resp.Errors, deleteObjectError{
				Key: key, VersionID: versionID,
				Code: "AccessDenied", Message: "access denied",
			})
			continue
		}
		bypass := false
		if headerBypass {
			if !h.Svc.AuthorizeRequest(bucket, key, "s3:BypassGovernanceRetention", creds.AccessKey, requestSourceIP(r), r.TLS != nil) {
				resp.Errors = append(resp.Errors, deleteObjectError{
					Key: key, VersionID: versionID,
					Code: "AccessDenied", Message: "not authorized to bypass governance retention",
				})
				continue
			}
			bypass = true
		}
		info, err := h.Svc.DeleteObjectResult(ctx, sk, key, versionID, DeleteOptions{BypassGovernance: bypass})
		if err != nil {
			if info.Missing && versionID == "" {
				if !req.Quiet {
					resp.Deleted = append(resp.Deleted, deletedObjectResult{Key: key})
				}
				continue
			}
			code, msg := deleteErrorCode(err)
			resp.Errors = append(resp.Errors, deleteObjectError{
				Key: key, VersionID: versionID, Code: code, Message: msg,
			})
			if err == metadata.ErrRetentionLocked || err == metadata.ErrLegalHold {
				h.emitActivity(r, creds.AccessKey, metadata.ActionObjectDeleteBlocked, "object", bucket+"/"+key)
			}
			continue
		}
		if !req.Quiet {
			resp.Deleted = append(resp.Deleted, deletedFromInfo(key, info))
		}
	}
	writeXML(w, http.StatusOK, resp)
}

func deletedFromInfo(key string, info ObjectDeleteInfo) deletedObjectResult {
	out := deletedObjectResult{Key: key, VersionID: info.VersionID}
	if info.DeleteMarker {
		t := true
		out.DeleteMarker = &t
		out.DeleteMarkerVersionID = info.DeleteMarkerVersionID
		if out.VersionID == "" {
			out.VersionID = info.DeleteMarkerVersionID
		}
	}
	return out
}

func deleteErrorCode(err error) (string, string) {
	switch {
	case errors.Is(err, metadata.ErrLegalHold):
		return "AccessDenied", "object is under legal hold"
	case errors.Is(err, metadata.ErrRetentionLocked):
		return "AccessDenied", "object retention period has not expired"
	case errors.Is(err, metadata.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		return "NoSuchVersion", "The specified version does not exist"
	default:
		return "InternalError", err.Error()
	}
}

func readDeleteBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, io.ErrUnexpectedEOF
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, deleteObjectsMaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > deleteObjectsMaxBody {
		return nil, io.ErrUnexpectedEOF
	}
	return body, nil
}

func validateDeleteChecksums(h http.Header, body []byte) error {
	saw := false
	if v := strings.TrimSpace(h.Get("Content-MD5")); v != "" {
		saw = true
		sum := md5.Sum(body)
		got, err := base64.StdEncoding.DecodeString(v)
		if err != nil || !bytes.Equal(got, sum[:]) {
			return errDeleteDigest
		}
	}
	for _, name := range []string{
		"x-amz-checksum-crc32",
		"x-amz-checksum-crc32c",
		"x-amz-checksum-sha1",
		"x-amz-checksum-sha256",
		"x-amz-checksum-crc64nvme",
	} {
		v := strings.TrimSpace(h.Get(name))
		if v == "" {
			continue
		}
		saw = true
		if !checksumHeaderMatches(name, body, v) {
			return errDeleteDigest
		}
	}
	if !saw {
		return errors.New("missing checksum")
	}
	return nil
}

func checksumHeaderMatches(name string, body []byte, b64 string) bool {
	var sum []byte
	switch name {
	case "x-amz-checksum-crc32":
		h := crc32.NewIEEE()
		_, _ = h.Write(body)
		sum = h.Sum(nil)
	case "x-amz-checksum-crc32c":
		h := crc32.New(crc32.MakeTable(crc32.Castagnoli))
		_, _ = h.Write(body)
		sum = h.Sum(nil)
	case "x-amz-checksum-sha1":
		d := sha1.Sum(body)
		sum = d[:]
	case "x-amz-checksum-sha256":
		d := sha256.Sum256(body)
		sum = d[:]
	case "x-amz-checksum-crc64nvme":
		h := crc64.New(crc64.MakeTable(crc64NVMEPoly))
		_, _ = h.Write(body)
		sum = h.Sum(nil)
	default:
		return false
	}
	got, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(got) != len(sum) {
		return false
	}
	return bytes.Equal(got, sum)
}

func governanceHeader(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("x-amz-bypass-governance-retention")), "true")
}

// governanceBypass reports whether this single-object DELETE may skip GOVERNANCE
// retention. A missing header is not a bypass. A header without
// s3:BypassGovernanceRetention is AccessDenied.
func (h *Handler) governanceBypass(w http.ResponseWriter, r *http.Request, creds auth.Credentials, bucket, key string) (bool, bool) {
	if !governanceHeader(r) {
		return false, true
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:BypassGovernanceRetention") {
		return false, false
	}
	return true, true
}
