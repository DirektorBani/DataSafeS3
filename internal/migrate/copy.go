package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrInterrupted is a cancelled copy. The state directory can be reused.
var ErrInterrupted = errors.New("interrupted")

// Run copies buckets from MinIO (or any S3 source) onto DataSafe and writes the report.
func Run(ctx context.Context, opt Options) (Evidence, error) {
	if opt.Concurrency < 1 {
		opt.Concurrency = 1
	}
	if opt.PartSize < 1 {
		opt.PartSize = defaultPartSize
	}
	ev := Evidence{
		StartedAt: time.Now().UTC(),
		Source:    opt.Source.URL,
		Tool:      "storage-cli migrate from-minio",
		Verdicts:  map[string]string{},
	}
	if len(opt.Targets) > 0 {
		ev.Target = opt.Targets[0].URL
	}
	st, err := OpenState(opt.StateDir)
	if err != nil {
		return ev, err
	}
	defer st.Close()
	r := &runner{opt: opt, state: st, ev: &ev, idx: map[string]*targetBucket{}}
	if err := r.pickLeader(ctx); err != nil && ctx.Err() != nil {
		ev.Interrupted = true
		ev.finish()
		_ = writeReport(opt.ReportDir, ev)
		return ev, ErrInterrupted
	}
	buckets, err := r.listBuckets(ctx)
	if err != nil {
		return r.fail(ev, err)
	}
	if len(opt.Buckets) > 0 {
		want := map[string]bool{}
		for _, b := range opt.Buckets {
			want[b] = true
		}
		filtered := buckets[:0]
		for _, b := range buckets {
			if want[b] {
				filtered = append(filtered, b)
			}
		}
		buckets = filtered
	}
	var groups [][]ObjectVersion
	for _, bucket := range buckets {
		if err := ctx.Err(); err != nil {
			ev.Interrupted = true
			break
		}
		warns, err := r.copyBucketConfig(ctx, bucket)
		ev.Warnings = append(ev.Warnings, warns...)
		if err != nil {
			ev.Errors = append(ev.Errors, bucket+": "+err.Error())
			var skipped configRefusal
			if !errors.As(err, &skipped) {
				continue
			}
		}
		if err := r.primeTarget(ctx, bucket); err != nil {
			ev.Errors = append(ev.Errors, bucket+" target list: "+err.Error())
			continue
		}
		vers, err := r.listVersions(ctx, opt.Source, bucket)
		if err != nil {
			ev.Errors = append(ev.Errors, bucket+" list: "+err.Error())
			continue
		}
		ordered := OrderVersions(vers)
		byKey := map[string][]ObjectVersion{}
		var order []string
		for _, v := range ordered {
			if _, ok := byKey[v.Key]; !ok {
				order = append(order, v.Key)
			}
			byKey[v.Key] = append(byKey[v.Key], v)
		}
		for _, key := range order {
			groups = append(groups, byKey[key])
		}
	}
	if !ev.Interrupted {
		if err := r.copyGroups(ctx, groups); err != nil {
			if errors.Is(err, context.Canceled) {
				ev.Interrupted = true
			} else {
				ev.Errors = append(ev.Errors, err.Error())
			}
		}
	}
	if ctx.Err() != nil {
		ev.Interrupted = true
	}
	if !ev.Interrupted {
		if err := r.verify(ctx, buckets); err != nil {
			ev.Errors = append(ev.Errors, "verify: "+err.Error())
		}
	}
	ev.finish()
	if err := writeReport(opt.ReportDir, ev); err != nil {
		return ev, err
	}
	if ev.Interrupted {
		return ev, ErrInterrupted
	}
	if !ev.OK() {
		return ev, fmt.Errorf("migration verification failed")
	}
	return ev, nil
}

func (r *runner) fail(ev Evidence, err error) (Evidence, error) {
	ev.Errors = append(ev.Errors, err.Error())
	ev.finish()
	_ = writeReport(r.opt.ReportDir, ev)
	return ev, err
}

type configRefusal struct{ msg string }

func (e configRefusal) Error() string { return e.msg }

type targetBucket struct {
	mu       sync.Mutex
	bySource map[string]string
	byETag   map[string]string
}

type runner struct {
	opt    Options
	state  *State
	ev     *Evidence
	mu     sync.Mutex
	target int
	done   int
	idx    map[string]*targetBucket
}

func (r *runner) current() Endpoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.opt.Targets) == 0 {
		return Endpoint{}
	}
	return r.opt.Targets[r.target%len(r.opt.Targets)]
}

func (r *runner) pickLeader(ctx context.Context) error {
	if len(r.opt.Targets) < 2 {
		return nil
	}
	for i, ep := range r.opt.Targets {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ep.URL, "/")+"/healthz", nil)
		if err != nil {
			continue
		}
		resp, err := ep.httpClient().Do(req)
		if err != nil {
			continue
		}
		var body struct {
			IsLeader *bool  `json:"is_leader"`
			Status   string `json:"status"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if body.IsLeader != nil && *body.IsLeader {
			r.mu.Lock()
			r.target = i
			r.mu.Unlock()
			r.opt.logf("target leader %s", ep.URL)
			return nil
		}
	}
	r.opt.logf("no leader in /healthz; using the first target")
	return nil
}

func (r *runner) call(ctx context.Context, ep Endpoint, method, path string, q url.Values, body io.Reader, hdr http.Header) (*http.Response, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}
	var last error
	for attempt := 0; attempt < 6; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		resp, err := ep.do(ctx, method, path, q, rdr, hdr)
		if err != nil {
			last = err
			if !sleepCtx(ctx, time.Duration(attempt+1)*200*time.Millisecond) {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			msg := readErr(resp)
			resp.Body.Close()
			if isNotLeader(resp.StatusCode, msg) {
				last = ErrNotLeader
				_ = r.pickLeader(ctx)
				if !sleepCtx(ctx, time.Duration(attempt+1)*200*time.Millisecond) {
					return nil, ctx.Err()
				}
				ep = r.current()
				continue
			}
			return nil, fmt.Errorf("503 %s", msg)
		}
		return resp, nil
	}
	if last == nil {
		last = ErrNotLeader
	}
	return nil, last
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (r *runner) source(ctx context.Context, method, path string, q url.Values, body io.Reader, hdr http.Header) (*http.Response, error) {
	return r.call(ctx, r.opt.Source, method, path, q, body, hdr)
}

func (r *runner) dest(ctx context.Context, method, path string, q url.Values, body io.Reader, hdr http.Header) (*http.Response, error) {
	return r.call(ctx, r.current(), method, path, q, body, hdr)
}

func (r *runner) listBuckets(ctx context.Context) ([]string, error) {
	resp, err := r.source(ctx, http.MethodGet, "/", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list buckets %d %s", resp.StatusCode, readErr(resp))
	}
	var parsed listBucketsResult
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	var names []string
	for _, b := range parsed.Buckets {
		if b.Name != "" {
			names = append(names, b.Name)
		}
	}
	return names, nil
}

func (r *runner) listVersions(ctx context.Context, ep Endpoint, bucket string) ([]ObjectVersion, error) {
	q := url.Values{"versions": {""}, "max-keys": {"1000"}}
	var out []ObjectVersion
	seenMarker := ""
	seq := 0
	for page := 0; page < 10000; page++ {
		resp, err := r.call(ctx, ep, http.MethodGet, "/"+url.PathEscape(bucket), q, nil, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			msg := readErr(resp)
			resp.Body.Close()
			return nil, fmt.Errorf("list versions %d %s", resp.StatusCode, msg)
		}
		truncated, nextKey, nextVid, items, nextSeq, err := decodeVersionPage(resp.Body, bucket, seq)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		seq = nextSeq
		out = append(out, items...)
		if !truncated {
			break
		}
		marker := nextKey + "\n" + nextVid
		if marker == seenMarker || (nextKey == "" && nextVid == "") {
			break
		}
		seenMarker = marker
		q.Set("key-marker", nextKey)
		q.Set("version-id-marker", nextVid)
	}
	return out, nil
}

func (r *runner) copyBucketConfig(ctx context.Context, bucket string) ([]string, error) {
	var warnings []string
	resp, err := r.dest(ctx, http.MethodPut, "/"+url.PathEscape(bucket), nil, nil, nil)
	if err != nil {
		return warnings, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return warnings, fmt.Errorf("create bucket %d", resp.StatusCode)
	}
	// Source encryption is not copied. An empty source configuration must not
	// delete encryption the target already has. Snapshot it and put it back
	// if a later config write cleared it.
	savedEnc, _ := r.getBody(ctx, r.current(), http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"encryption": {""}})
	if status, err := r.getVersioning(ctx, r.opt.Source, bucket); err == nil && status != "" {
		body := "<VersioningConfiguration xmlns=\"http://s3.amazonaws.com/doc/2006-03-01/\"><Status>" + xmlEscape(status) + "</Status></VersioningConfiguration>"
		if err := r.putXML(ctx, bucket, url.Values{"versioning": {""}}, body); err != nil {
			warnings = append(warnings, bucket+" versioning: "+err.Error())
		}
	}
	if xmlBody, err := r.getBody(ctx, r.opt.Source, http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"object-lock": {""}}); err == nil && strings.TrimSpace(xmlBody) != "" {
		if err := r.putXML(ctx, bucket, url.Values{"object-lock": {""}}, xmlBody); err != nil {
			warnings = append(warnings, bucket+" object lock: "+err.Error())
		}
	}
	if xmlBody, err := r.getBody(ctx, r.opt.Source, http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"lifecycle": {""}}); err == nil && strings.TrimSpace(xmlBody) != "" {
		if err := LifecycleUnsupported(xmlBody); err != nil {
			warnings = append(warnings, r.restoreTargetEncryption(ctx, bucket, savedEnc)...)
			return warnings, configRefusal{"lifecycle: " + err.Error()}
		}
		if err := r.putXML(ctx, bucket, url.Values{"lifecycle": {""}}, xmlBody); err != nil {
			warnings = append(warnings, r.restoreTargetEncryption(ctx, bucket, savedEnc)...)
			return warnings, configRefusal{"lifecycle: " + err.Error()}
		}
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"policy": {""}}); err == nil && strings.TrimSpace(raw) != "" {
		ported, warns := PortPolicy(raw)
		for _, w := range warns {
			warnings = append(warnings, bucket+": "+w)
		}
		if ported != "" {
			resp, err := r.dest(ctx, http.MethodPut, "/"+url.PathEscape(bucket), url.Values{"policy": {""}}, strings.NewReader(ported), nil)
			if err != nil {
				warnings = append(warnings, bucket+" policy: "+err.Error())
			} else {
				msg := ""
				if resp.StatusCode >= 300 {
					msg = readErr(resp)
				}
				resp.Body.Close()
				if resp.StatusCode >= 300 {
					warnings = append(warnings, bucket+" policy: "+msg)
				}
			}
		}
	}
	warnings = append(warnings, r.restoreTargetEncryption(ctx, bucket, savedEnc)...)
	return warnings, nil
}

func (r *runner) restoreTargetEncryption(ctx context.Context, bucket, savedEnc string) []string {
	if !strings.Contains(savedEnc, "<SSEAlgorithm>") {
		return nil
	}
	now, err := r.getBody(ctx, r.current(), http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"encryption": {""}})
	if err == nil && strings.Contains(now, "<SSEAlgorithm>") {
		return nil
	}
	if putErr := r.putXML(ctx, bucket, url.Values{"encryption": {""}}, savedEnc); putErr != nil {
		return []string{bucket + " encryption restore: " + putErr.Error()}
	}
	return nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (r *runner) getVersioning(ctx context.Context, ep Endpoint, bucket string) (string, error) {
	raw, err := r.getBody(ctx, ep, http.MethodGet, "/"+url.PathEscape(bucket), url.Values{"versioning": {""}})
	if err != nil {
		return "", err
	}
	var cfg struct {
		Status string `xml:"Status"`
	}
	if err := xml.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", err
	}
	return cfg.Status, nil
}

func (r *runner) getBody(ctx context.Context, ep Endpoint, method, path string, q url.Values) (string, error) {
	resp, err := r.call(ctx, ep, method, path, q, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func (r *runner) putXML(ctx context.Context, bucket string, q url.Values, body string) error {
	resp, err := r.dest(ctx, http.MethodPut, "/"+url.PathEscape(bucket), q, strings.NewReader(body), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%d %s", resp.StatusCode, readErr(resp))
	}
	return nil
}

func (r *runner) copyGroups(ctx context.Context, groups [][]ObjectVersion) error {
	sem := make(chan struct{}, r.opt.Concurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, len(groups))
	for _, g := range groups {
		if err := ctx.Err(); err != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(group []ObjectVersion) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, v := range group {
				if err := ctx.Err(); err != nil {
					errCh <- err
					return
				}
				if err := r.copyVersion(ctx, v); err != nil {
					errCh <- fmt.Errorf("%s/%s: %w", v.Bucket, v.Key, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	var errs []string
	for err := range errCh {
		if errors.Is(err, context.Canceled) {
			return err
		}
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (r *runner) copyVersion(ctx context.Context, v ObjectVersion) error {
	id := versionIdentity(v)
	if r.state.Has(v.Bucket, v.Key, id) {
		r.progress(v, "skipped")
		return nil
	}
	known := r.lookupSource(v.Bucket, v.Key, id)
	skipBytes := known != ""
	if !skipBytes && !r.opt.SSEActive() {
		if vid := r.lookupETag(v); vid != "" {
			skipBytes = true
			known = vid
		}
	}
	var newID string
	if !skipBytes {
		if v.DeleteMarker {
			hdr := http.Header{}
			hdr.Set(sourceVersionMeta, id)
			resp, err := r.dest(ctx, http.MethodDelete, objectPath(v.Bucket, v.Key), nil, nil, hdr)
			if err != nil {
				return err
			}
			newID = resp.Header.Get("x-amz-version-id")
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				return fmt.Errorf("delete marker %d", resp.StatusCode)
			}
		} else {
			vid, err := r.copyBytes(ctx, v, id)
			if err != nil {
				return err
			}
			newID = vid
		}
		r.remember(v, id, newID)
	}
	// Side data is applied even when the bytes were already there, so a crash
	// between PUT and retention still sets the lock on resume.
	if !v.DeleteMarker {
		vid := newID
		if vid == "" {
			vid = known
		}
		if err := r.copySideData(ctx, v, vid); err != nil {
			return err
		}
	}
	if !skipBytes {
		if err := r.throttle(ctx, v.Size); err != nil {
			return err
		}
	}
	status := "copied"
	if skipBytes {
		status = "skipped"
	}
	if err := r.state.Mark(v.Bucket, v.Key, id, status, v.ETag, v.Size); err != nil {
		return err
	}
	r.progress(v, status)
	return nil
}

func (o Options) SSEActive() bool {
	return strings.EqualFold(o.SSE, "aws:kms") || strings.EqualFold(o.SSE, "AES256")
}

func (r *runner) progress(v ObjectVersion, status string) {
	r.mu.Lock()
	r.done++
	n := r.done
	r.mu.Unlock()
	r.opt.logf("%s %s/%s %s", status, v.Bucket, v.Key, versionIdentity(v))
	if r.opt.AfterObject != nil {
		r.opt.AfterObject(n)
	}
}

func (r *runner) throttle(ctx context.Context, n int64) error {
	if r.opt.RateBytes <= 0 || n <= 0 {
		return nil
	}
	d := time.Duration(float64(n) / float64(r.opt.RateBytes) * float64(time.Second))
	if d <= 0 {
		return nil
	}
	if !sleepCtx(ctx, d) {
		return ctx.Err()
	}
	return nil
}

func (r *runner) copyBytes(ctx context.Context, v ObjectVersion, id string) (string, error) {
	q := url.Values{}
	if v.VersionID != "" {
		q.Set("versionId", v.VersionID)
	}
	resp, err := r.source(ctx, http.MethodGet, objectPath(v.Bucket, v.Key), q, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("get %d %s", resp.StatusCode, readErr(resp))
	}
	hdr := http.Header{}
	hdr.Set(sourceVersionMeta, id)
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		hdr.Set("Content-Type", ct)
	}
	for k, vs := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") && !strings.EqualFold(k, sourceVersionMeta) && len(vs) > 0 {
			hdr.Set(k, vs[0])
		}
	}
	if strings.EqualFold(r.opt.SSE, "aws:kms") {
		hdr.Set("x-amz-server-side-encryption", "aws:kms")
		hdr.Set("x-amz-server-side-encryption-aws-kms-key-id", r.opt.KMSKey)
	} else if strings.EqualFold(r.opt.SSE, "AES256") {
		hdr.Set("x-amz-server-side-encryption", "AES256")
	}
	if v.Size > r.opt.PartSize {
		return r.multipart(ctx, v, hdr, resp.Body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	put, err := r.dest(ctx, http.MethodPut, objectPath(v.Bucket, v.Key), nil, bytes.NewReader(body), hdr)
	if err != nil {
		return "", err
	}
	defer put.Body.Close()
	if put.StatusCode >= 300 {
		return "", fmt.Errorf("put %d %s", put.StatusCode, readErr(put))
	}
	return put.Header.Get("x-amz-version-id"), nil
}

func (r *runner) multipart(ctx context.Context, v ObjectVersion, hdr http.Header, body io.Reader) (string, error) {
	init, err := r.dest(ctx, http.MethodPost, objectPath(v.Bucket, v.Key), url.Values{"uploads": {""}}, nil, hdr)
	if err != nil {
		return "", err
	}
	raw, _ := io.ReadAll(init.Body)
	init.Body.Close()
	if init.StatusCode >= 300 {
		return "", fmt.Errorf("multipart init %d %s", init.StatusCode, strings.TrimSpace(string(raw)))
	}
	var started struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(raw, &started); err != nil || started.UploadID == "" {
		return "", fmt.Errorf("multipart init: %s", strings.TrimSpace(string(raw)))
	}
	var parts []struct {
		Number int
		ETag   string
	}
	buf := make([]byte, r.opt.PartSize)
	for n := 1; ; n++ {
		read, err := io.ReadFull(body, buf)
		if read > 0 {
			q := url.Values{"uploadId": {started.UploadID}, "partNumber": {fmt.Sprint(n)}}
			resp, err := r.dest(ctx, http.MethodPut, objectPath(v.Bucket, v.Key), q, bytes.NewReader(buf[:read]), nil)
			if err != nil {
				return "", err
			}
			if resp.StatusCode >= 300 {
				msg := readErr(resp)
				resp.Body.Close()
				return "", fmt.Errorf("part %d %s", n, msg)
			}
			resp.Body.Close()
			parts = append(parts, struct {
				Number int
				ETag   string
			}{n, resp.Header.Get("ETag")})
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	var complete strings.Builder
	complete.WriteString(`<CompleteMultipartUpload>`)
	for _, p := range parts {
		fmt.Fprintf(&complete, `<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>`, p.Number, xmlEscape(p.ETag))
	}
	complete.WriteString(`</CompleteMultipartUpload>`)
	resp, err := r.dest(ctx, http.MethodPost, objectPath(v.Bucket, v.Key), url.Values{"uploadId": {started.UploadID}}, strings.NewReader(complete.String()), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("complete multipart %d %s", resp.StatusCode, readErr(resp))
	}
	return resp.Header.Get("x-amz-version-id"), nil
}

func (r *runner) copySideData(ctx context.Context, v ObjectVersion, targetVersion string) error {
	q := url.Values{}
	if v.VersionID != "" {
		q.Set("versionId", v.VersionID)
	}
	dstVersion := func(sub string) url.Values {
		dst := url.Values{sub: {""}}
		if targetVersion != "" {
			dst.Set("versionId", targetVersion)
		}
		return dst
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "retention", "")); err == nil && strings.Contains(raw, "RetainUntilDate") {
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dstVersion("retention"), raw); err != nil {
			return err
		}
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "legal-hold", "")); err == nil && strings.Contains(raw, ">ON<") {
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dstVersion("legal-hold"), raw); err != nil {
			return err
		}
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "tagging", "")); err == nil && strings.Contains(raw, "<Tag>") {
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dstVersion("tagging"), raw); err != nil {
			return err
		}
	}
	return nil
}

func mergeQuery(base url.Values, key, val string) url.Values {
	q := url.Values{}
	for k, vs := range base {
		q[k] = append([]string(nil), vs...)
	}
	q.Set(key, val)
	return q
}

func (r *runner) putRaw(ctx context.Context, path string, q url.Values, body string) error {
	resp, err := r.dest(ctx, http.MethodPut, path, q, strings.NewReader(body), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%d %s", resp.StatusCode, readErr(resp))
	}
	return nil
}

func (r *runner) primeTarget(ctx context.Context, bucket string) error {
	vers, err := r.listVersions(ctx, r.current(), bucket)
	if err != nil {
		return err
	}
	idx := &targetBucket{bySource: map[string]string{}, byETag: map[string]string{}}
	for _, tv := range vers {
		q := url.Values{}
		if tv.VersionID != "" {
			q.Set("versionId", tv.VersionID)
		}
		resp, err := r.dest(ctx, http.MethodHead, objectPath(bucket, tv.Key), q, nil, nil)
		if err != nil {
			return err
		}
		src := resp.Header.Get(sourceVersionMeta)
		resp.Body.Close()
		if src != "" {
			idx.bySource[tv.Key+"\n"+src] = tv.VersionID
		}
		idx.byETag[etagIndexKey(tv)] = tv.VersionID
	}
	r.mu.Lock()
	if r.idx == nil {
		r.idx = map[string]*targetBucket{}
	}
	r.idx[bucket] = idx
	r.mu.Unlock()
	return nil
}

func etagIndexKey(v ObjectVersion) string {
	kind := "v"
	if v.DeleteMarker {
		kind = "d"
	}
	return v.Key + "\n" + trimETag(v.ETag) + "\n" + fmt.Sprint(v.Size) + "\n" + kind
}

func (r *runner) bucketIndex(bucket string) *targetBucket {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.idx[bucket]
}

func (r *runner) lookupSource(bucket, key, sourceID string) string {
	idx := r.bucketIndex(bucket)
	if idx == nil {
		return ""
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.bySource[key+"\n"+sourceID]
}

func (r *runner) lookupETag(v ObjectVersion) string {
	if v.ETag == "" {
		return ""
	}
	idx := r.bucketIndex(v.Bucket)
	if idx == nil {
		return ""
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.byETag[etagIndexKey(v)]
}

func (r *runner) remember(v ObjectVersion, sourceID, targetID string) {
	idx := r.bucketIndex(v.Bucket)
	if idx == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if sourceID != "" {
		idx.bySource[v.Key+"\n"+sourceID] = targetID
	}
	copied := v
	idx.byETag[etagIndexKey(copied)] = targetID
}

func (r *runner) verify(ctx context.Context, buckets []string) error {
	for _, bucket := range buckets {
		src, err := r.listVersions(ctx, r.opt.Source, bucket)
		if err != nil {
			return err
		}
		dst, err := r.listVersions(ctx, r.current(), bucket)
		if err != nil {
			return err
		}
		be := BucketEvidence{Name: bucket}
		for _, v := range src {
			if v.DeleteMarker {
				be.SourceMarkers++
				continue
			}
			be.SourceObjects++
			be.SourceBytes += v.Size
		}
		for _, v := range dst {
			if v.DeleteMarker {
				be.TargetMarkers++
				continue
			}
			be.TargetObjects++
			be.TargetBytes += v.Size
		}
		if markerChain(src) != markerChain(dst) {
			be.MarkerChainMismatch = 1
		}
		dstByKey := map[string][]ObjectVersion{}
		for _, v := range dst {
			dstByKey[v.Key] = append(dstByKey[v.Key], v)
		}
		bySource := r.indexTarget(ctx, bucket, dst)
		var objects []ObjectVersion
		for _, v := range src {
			if !v.DeleteMarker {
				objects = append(objects, v)
			}
		}
		samples := SampleIndexes(len(objects), r.opt.VerifySample, r.opt.VerifyFull)
		for i, v := range objects {
			sample := false
			if _, ok := samples[i]; ok {
				sample = true
			}
			match, ok := bySource[v.Key+"\n"+versionIdentity(v)]
			if !ok {
				found := matchVersion(v, dstByKey[v.Key], r.opt.SSEActive())
				if found == nil {
					be.ETagMismatches++
					if sample {
						be.SampleChecked++
						be.SampleMismatches++
					}
					continue
				}
				match = *found
			}
			if !r.etagOK(ctx, v, match) {
				be.ETagMismatches++
			}
			if err := r.compareLock(ctx, v, match.VersionID); err != nil {
				be.LockMismatches++
			}
			if sample {
				be.SampleChecked++
				if err := r.compareSHA(ctx, v, match.VersionID); err != nil {
					be.SampleMismatches++
				}
			}
		}
		r.mu.Lock()
		r.ev.Buckets = append(r.ev.Buckets, be)
		r.mu.Unlock()
	}
	return nil
}

func matchVersion(src ObjectVersion, dst []ObjectVersion, sse bool) *ObjectVersion {
	for i := range dst {
		if dst[i].DeleteMarker {
			continue
		}
		if !sse && src.ETag != "" && dst[i].ETag == src.ETag && dst[i].Size == src.Size {
			return &dst[i]
		}
		if sse && dst[i].Size == src.Size {
			return &dst[i]
		}
	}
	if sse {
		for i := range dst {
			if !dst[i].DeleteMarker {
				return &dst[i]
			}
		}
	}
	return nil
}

func markerChain(vers []ObjectVersion) string {
	ordered := OrderVersions(vers)
	var b strings.Builder
	last := ""
	for _, v := range ordered {
		if v.Key != last {
			if last != "" {
				b.WriteByte('\n')
			}
			b.WriteString(v.Key)
			b.WriteByte('=')
			last = v.Key
		}
		if v.DeleteMarker {
			b.WriteByte('D')
		} else {
			b.WriteByte('V')
		}
	}
	return b.String()
}

// SampleIndexes picks objects to hash. A positive percent checks at least one
// object, including a bucket smaller than the step size.
func SampleIndexes(total int, percent float64, full bool) map[int]struct{} {
	out := map[int]struct{}{}
	if total <= 0 {
		return out
	}
	if full {
		for i := 0; i < total; i++ {
			out[i] = struct{}{}
		}
		return out
	}
	if percent <= 0 {
		return out
	}
	want := int(percent / 100 * float64(total))
	if want < 1 {
		want = 1
	}
	if want > total {
		want = total
	}
	if want == 1 || want == total {
		if want == total {
			for i := 0; i < total; i++ {
				out[i] = struct{}{}
			}
			return out
		}
		out[0] = struct{}{}
		return out
	}
	for i := 0; i < want; i++ {
		out[i*(total-1)/(want-1)] = struct{}{}
	}
	return out
}

func (r *runner) compareLock(ctx context.Context, src ObjectVersion, dstVersion string) error {
	q := url.Values{}
	if src.VersionID != "" {
		q.Set("versionId", src.VersionID)
	}
	raw, srcErr := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(src.Bucket, src.Key), mergeQuery(q, "retention", ""))
	dq := url.Values{"retention": {""}}
	if dstVersion != "" {
		dq.Set("versionId", dstVersion)
	}
	got, dstErr := r.getBody(ctx, r.current(), http.MethodGet, objectPath(src.Bucket, src.Key), dq)
	srcMode, srcUntil, srcOn := retentionView(raw)
	dstMode, dstUntil, dstOn := retentionView(got)
	if srcErr != nil {
		srcOn = false
	}
	if dstErr != nil {
		dstOn = false
	}
	if srcOn != dstOn || (srcOn && (!strings.EqualFold(srcMode, dstMode) || !sameInstant(srcUntil, dstUntil))) {
		return fmt.Errorf("retention mismatch")
	}
	srcHold, _ := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(src.Bucket, src.Key), mergeQuery(q, "legal-hold", ""))
	hq := url.Values{"legal-hold": {""}}
	if dstVersion != "" {
		hq.Set("versionId", dstVersion)
	}
	dstHold, _ := r.getBody(ctx, r.current(), http.MethodGet, objectPath(src.Bucket, src.Key), hq)
	if holdOn(srcHold) != holdOn(dstHold) {
		return fmt.Errorf("legal hold mismatch")
	}
	return nil
}

func retentionView(raw string) (mode, until string, present bool) {
	dec := xml.NewDecoder(strings.NewReader(raw))
	var stack []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
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
				mode = text
			case "RetainUntilDate":
				until = text
				present = true
			}
		}
	}
	return mode, until, present
}

func sameInstant(a, b string) bool {
	if a == "" && b == "" {
		return true
	}
	ta, ea := parseTimeOK(a)
	tb, eb := parseTimeOK(b)
	if ea != nil || eb != nil {
		return a == b
	}
	return ta.UTC().Truncate(time.Second).Equal(tb.UTC().Truncate(time.Second))
}

func parseTimeOK(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("time")
}

func holdOn(raw string) bool {
	return strings.Contains(raw, ">ON<")
}

func (r *runner) compareSHA(ctx context.Context, src ObjectVersion, dstVersion string) error {
	sum := func(ep Endpoint, version string) (string, error) {
		q := url.Values{}
		if version != "" {
			q.Set("versionId", version)
		}
		resp, err := r.call(ctx, ep, http.MethodGet, objectPath(src.Bucket, src.Key), q, nil, nil)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("get %d", resp.StatusCode)
		}
		h := sha256.New()
		if _, err := io.Copy(h, resp.Body); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	a, err := sum(r.opt.Source, src.VersionID)
	if err != nil {
		return err
	}
	b, err := sum(r.current(), dstVersion)
	if err != nil {
		return err
	}
	if a != b {
		return fmt.Errorf("sha mismatch")
	}
	return nil
}

func (r *runner) indexTarget(ctx context.Context, bucket string, vers []ObjectVersion) map[string]ObjectVersion {
	out := map[string]ObjectVersion{}
	for _, tv := range vers {
		if tv.DeleteMarker {
			continue
		}
		q := url.Values{}
		if tv.VersionID != "" {
			q.Set("versionId", tv.VersionID)
		}
		resp, err := r.call(ctx, r.current(), http.MethodHead, objectPath(bucket, tv.Key), q, nil, nil)
		if err != nil {
			continue
		}
		src := resp.Header.Get(sourceVersionMeta)
		resp.Body.Close()
		if src != "" {
			out[tv.Key+"\n"+src] = tv
		}
	}
	return out
}

func (r *runner) etagOK(ctx context.Context, src ObjectVersion, match ObjectVersion) bool {
	if src.ETag == "" || trimETag(src.ETag) == trimETag(match.ETag) {
		return true
	}
	srcE, dstE := trimETag(src.ETag), trimETag(match.ETag)
	if strings.Contains(srcE, "-") || strings.Contains(dstE, "-") {
		body, err := r.objectBytes(ctx, r.current(), src.Bucket, src.Key, match.VersionID)
		if err == nil && ETagsAgree(src.ETag, match.ETag, body, r.opt.PartSize) {
			return true
		}
	}
	// SSE-KMS and SSE-S3 store an ETag that is not the plaintext MD5.
	// The bytes are still the object. Compare them.
	a, errA := r.objectSHA(ctx, r.opt.Source, src.Bucket, src.Key, src.VersionID)
	b, errB := r.objectSHA(ctx, r.current(), src.Bucket, src.Key, match.VersionID)
	return errA == nil && errB == nil && a == b
}

func (r *runner) objectSHA(ctx context.Context, ep Endpoint, bucket, key, version string) (string, error) {
	body, err := r.objectBytes(ctx, ep, bucket, key, version)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func (r *runner) objectBytes(ctx context.Context, ep Endpoint, bucket, key, version string) ([]byte, error) {
	q := url.Values{}
	if version != "" {
		q.Set("versionId", version)
	}
	resp, err := r.call(ctx, ep, http.MethodGet, objectPath(bucket, key), q, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
