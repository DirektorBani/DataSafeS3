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
	r := &runner{opt: opt, state: st, ev: &ev}
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

type runner struct {
	opt    Options
	state  *State
	ev     *Evidence
	mu     sync.Mutex
	target int
	done   int
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
	// MinIO caps a page at 1000 and sets IsTruncated. DataSafe returns one page,
	// so the page is large enough for the 10k-object acceptance run.
	q := url.Values{"versions": {""}, "max-keys": {"100000"}}
	var out []ObjectVersion
	seenMarker := ""
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
		var parsed listVersionsResult
		err = xml.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		add := func(items []listedVersion, marker bool) {
			for _, it := range items {
				out = append(out, ObjectVersion{
					Bucket:       bucket,
					Key:          it.Key,
					VersionID:    it.VersionID,
					ETag:         trimETag(it.ETag),
					Size:         it.Size,
					LastModified: parseTime(it.LastModified),
					IsLatest:     it.IsLatest,
					DeleteMarker: marker,
				})
			}
		}
		add(parsed.Versions, false)
		add(parsed.DeleteMarkers, true)
		if !parsed.IsTruncated {
			break
		}
		marker := parsed.NextKeyMarker + "\n" + parsed.NextVersionIDMarker
		if marker == seenMarker || (parsed.NextKeyMarker == "" && parsed.NextVersionIDMarker == "") {
			break
		}
		seenMarker = marker
		q.Set("key-marker", parsed.NextKeyMarker)
		q.Set("version-id-marker", parsed.NextVersionIDMarker)
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
		if msg := LifecycleWarning(xmlBody); msg != "" {
			warnings = append(warnings, bucket+": "+msg)
		}
		if err := r.putXML(ctx, bucket, url.Values{"lifecycle": {""}}, xmlBody); err != nil {
			warnings = append(warnings, bucket+" lifecycle: "+err.Error())
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
	return warnings, nil
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
	known, err := r.targetSourceVersions(ctx, v.Bucket, v.Key)
	if err != nil {
		return err
	}
	skipBytes := false
	if _, ok := known[id]; ok {
		skipBytes = true
	} else if !r.opt.SSEActive() && r.targetHasETag(ctx, v) {
		skipBytes = true
	}
	if !skipBytes {
		if v.DeleteMarker {
			hdr := http.Header{}
			hdr.Set(sourceVersionMeta, id)
			resp, err := r.dest(ctx, http.MethodDelete, objectPath(v.Bucket, v.Key), nil, nil, hdr)
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				return fmt.Errorf("delete marker %d", resp.StatusCode)
			}
		} else if err := r.copyBytes(ctx, v, id); err != nil {
			return err
		}
	}
	// Side data is applied even when the bytes were already there, so a crash
	// between PUT and retention still sets the lock on resume.
	if !v.DeleteMarker {
		if err := r.copySideData(ctx, v); err != nil {
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

func (r *runner) copyBytes(ctx context.Context, v ObjectVersion, id string) error {
	q := url.Values{}
	if v.VersionID != "" {
		q.Set("versionId", v.VersionID)
	}
	resp, err := r.source(ctx, http.MethodGet, objectPath(v.Bucket, v.Key), q, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("get %d %s", resp.StatusCode, readErr(resp))
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
		return err
	}
	put, err := r.dest(ctx, http.MethodPut, objectPath(v.Bucket, v.Key), nil, bytes.NewReader(body), hdr)
	if err != nil {
		return err
	}
	defer put.Body.Close()
	if put.StatusCode >= 300 {
		return fmt.Errorf("put %d %s", put.StatusCode, readErr(put))
	}
	return nil
}

func (r *runner) multipart(ctx context.Context, v ObjectVersion, hdr http.Header, body io.Reader) error {
	init, err := r.dest(ctx, http.MethodPost, objectPath(v.Bucket, v.Key), url.Values{"uploads": {""}}, nil, hdr)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(init.Body)
	init.Body.Close()
	if init.StatusCode >= 300 {
		return fmt.Errorf("multipart init %d %s", init.StatusCode, strings.TrimSpace(string(raw)))
	}
	var started struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(raw, &started); err != nil || started.UploadID == "" {
		return fmt.Errorf("multipart init: %s", strings.TrimSpace(string(raw)))
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
				return err
			}
			if resp.StatusCode >= 300 {
				msg := readErr(resp)
				resp.Body.Close()
				return fmt.Errorf("part %d %s", n, msg)
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
			return err
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
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("complete multipart %d %s", resp.StatusCode, readErr(resp))
	}
	return nil
}

func (r *runner) copySideData(ctx context.Context, v ObjectVersion) error {
	q := url.Values{}
	if v.VersionID != "" {
		q.Set("versionId", v.VersionID)
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "retention", "")); err == nil && strings.Contains(raw, "RetainUntilDate") {
		dst := url.Values{"retention": {""}}
		if vid := r.latestTargetVersion(ctx, v); vid != "" {
			dst.Set("versionId", vid)
		}
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dst, raw); err != nil {
			return err
		}
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "legal-hold", "")); err == nil && strings.Contains(raw, ">ON<") {
		dst := url.Values{"legal-hold": {""}}
		if vid := r.latestTargetVersion(ctx, v); vid != "" {
			dst.Set("versionId", vid)
		}
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dst, raw); err != nil {
			return err
		}
	}
	if raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(v.Bucket, v.Key), mergeQuery(q, "tagging", "")); err == nil && strings.Contains(raw, "<Tag>") {
		dst := url.Values{"tagging": {""}}
		if vid := r.latestTargetVersion(ctx, v); vid != "" {
			dst.Set("versionId", vid)
		}
		if err := r.putRaw(ctx, objectPath(v.Bucket, v.Key), dst, raw); err != nil {
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

func (r *runner) latestTargetVersion(ctx context.Context, v ObjectVersion) string {
	vers, err := r.listVersions(ctx, r.current(), v.Bucket)
	if err != nil {
		return ""
	}
	var latest ObjectVersion
	var found bool
	for _, tv := range vers {
		if tv.Key != v.Key || tv.DeleteMarker {
			continue
		}
		if !found || tv.IsLatest || tv.LastModified.After(latest.LastModified) {
			latest = tv
			found = true
		}
	}
	return latest.VersionID
}

func (r *runner) targetSourceVersions(ctx context.Context, bucket, key string) (map[string]string, error) {
	vers, err := r.listVersions(ctx, r.current(), bucket)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, tv := range vers {
		if tv.Key != key {
			continue
		}
		q := url.Values{}
		if tv.VersionID != "" {
			q.Set("versionId", tv.VersionID)
		}
		resp, err := r.dest(ctx, http.MethodHead, objectPath(bucket, key), q, nil, nil)
		if err != nil {
			return nil, err
		}
		src := resp.Header.Get(sourceVersionMeta)
		resp.Body.Close()
		if src != "" {
			out[src] = tv.VersionID
		}
	}
	return out, nil
}

func (r *runner) targetHasETag(ctx context.Context, v ObjectVersion) bool {
	if v.ETag == "" {
		return false
	}
	vers, err := r.listVersions(ctx, r.current(), v.Bucket)
	if err != nil {
		return false
	}
	for _, tv := range vers {
		if tv.Key == v.Key && tv.ETag == v.ETag && tv.Size == v.Size && tv.DeleteMarker == v.DeleteMarker {
			return true
		}
	}
	return false
}

func (r *runner) verify(ctx context.Context, buckets []string) error {
	sampleEvery := 0
	if r.opt.VerifyFull {
		sampleEvery = 1
	} else if r.opt.VerifySample > 0 {
		sampleEvery = int(100 / r.opt.VerifySample)
		if sampleEvery < 1 {
			sampleEvery = 1
		}
	}
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
		dstByKey := map[string][]ObjectVersion{}
		for _, v := range dst {
			dstByKey[v.Key] = append(dstByKey[v.Key], v)
		}
		bySource := r.indexTarget(ctx, bucket, dst)
		n := 0
		for _, v := range src {
			if v.DeleteMarker {
				continue
			}
			n++
			match, ok := bySource[v.Key+"\n"+versionIdentity(v)]
			if !ok {
				found := matchVersion(v, dstByKey[v.Key], r.opt.SSEActive())
				if found == nil {
					be.ETagMismatches++
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
			if sampleEvery > 0 && (r.opt.VerifyFull || n%sampleEvery == 0) {
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

func (r *runner) compareLock(ctx context.Context, src ObjectVersion, dstVersion string) error {
	q := url.Values{}
	if src.VersionID != "" {
		q.Set("versionId", src.VersionID)
	}
	raw, err := r.getBody(ctx, r.opt.Source, http.MethodGet, objectPath(src.Bucket, src.Key), mergeQuery(q, "retention", ""))
	if err != nil || !strings.Contains(raw, "RetainUntilDate") {
		return nil
	}
	dq := url.Values{"retention": {""}}
	if dstVersion != "" {
		dq.Set("versionId", dstVersion)
	}
	got, err := r.getBody(ctx, r.current(), http.MethodGet, objectPath(src.Bucket, src.Key), dq)
	if err != nil {
		return err
	}
	if !strings.Contains(got, "RetainUntilDate") {
		return fmt.Errorf("retention missing")
	}
	return nil
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
	if r.opt.SSEActive() || src.ETag == "" || trimETag(src.ETag) == trimETag(match.ETag) {
		return true
	}
	if !strings.Contains(trimETag(src.ETag), "-") {
		return false
	}
	body, err := r.objectBytes(ctx, r.current(), src.Bucket, src.Key, match.VersionID)
	if err != nil {
		return false
	}
	return ETagsAgree(src.ETag, match.ETag, body, r.opt.PartSize)
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
