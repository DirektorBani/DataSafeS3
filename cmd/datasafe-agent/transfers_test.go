package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

type fakeRemote struct {
	mu               sync.Mutex
	partNums         []int
	aborts           []string
	initiates        int
	completes        int
	puts             int
	netLeft          int
	notFoundOnce     bool
	etagFailOnce     bool
	block            chan struct{}
	data             []byte
	forceETag        string
	noRange          bool
	openOffsets      []int64
	keys             []string
	completeDelay    time.Duration
	completeFailLeft int
	completeStatus   int
	completeMissing  bool
	statFound        bool
	statSize         int64
	statETag         string
	statMiss         int
	openStatus       int
	openFails        int
	openErr          error
	gate             chan struct{}
	inflight         int
	maxInflight      int
	partsLive        int
	earlyAborts      int
	abortFailLeft    int
	abortTries       []string
	partHold         chan struct{}
	holding          bool
	unauthorized     bool
}

func (f *fakeRemote) Put(ctx context.Context, key string, body io.Reader, n int64) (string, error) {
	_, _ = io.Copy(io.Discard, body)
	f.mu.Lock()
	f.puts++
	f.keys = append(f.keys, key)
	if f.netLeft > 0 {
		f.netLeft--
		f.mu.Unlock()
		return "", errNetwork
	}
	f.inflight++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			f.mu.Lock()
			f.inflight--
			f.mu.Unlock()
			return "", ctx.Err()
		}
	}
	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
	return "put", nil
}

func (f *fakeRemote) maxPuts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInflight
}

func (f *fakeRemote) Initiate(ctx context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.initiates++
	return "up1", nil
}

func (f *fakeRemote) Part(ctx context.Context, key, uploadID string, part int, body io.Reader, n int64) (string, error) {
	_, _ = io.Copy(io.Discard, body)
	f.mu.Lock()
	f.partNums = append(f.partNums, part)
	f.partsLive++
	if f.netLeft > 0 {
		f.netLeft--
		f.partsLive--
		f.mu.Unlock()
		return "", errNetwork
	}
	if f.unauthorized {
		f.partsLive--
		f.mu.Unlock()
		return "", &apiFault{Status: 401, Code: "auth.session_expired", Body: `{"error":"session expired"}`}
	}
	if f.notFoundOnce {
		f.notFoundOnce = false
		f.partsLive--
		f.mu.Unlock()
		return "", &apiFault{Status: 404, Code: "multipart.upload_not_found", Body: "upload not found"}
	}
	block := f.block
	hold := f.partHold
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.partsLive--
		f.mu.Unlock()
	}()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			if hold != nil {
				f.mu.Lock()
				f.holding = true
				f.mu.Unlock()
				<-hold
			}
			return "", ctx.Err()
		}
	}
	return "e" + itoa(part), nil
}

func (f *fakeRemote) Complete(ctx context.Context, key, uploadID string, parts []partRec) (string, error) {
	f.mu.Lock()
	f.completes++
	if f.etagFailOnce {
		f.etagFailOnce = false
		f.mu.Unlock()
		return "", &apiFault{Status: 400, Body: "etag mismatch"}
	}
	delay := f.completeDelay
	if f.completeFailLeft > 0 {
		f.completeFailLeft--
		status := f.completeStatus
		f.mu.Unlock()
		if status >= 400 {
			return "", &apiFault{Status: status, Body: "complete rejected"}
		}
		return "", fmt.Errorf("%w: timeout awaiting response headers", context.DeadlineExceeded)
	}
	missing := f.completeMissing
	f.mu.Unlock()
	if missing {
		return "", &apiFault{Status: 404, Code: "multipart.upload_not_found", Body: "upload not found"}
	}
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "done", nil
}

func (f *fakeRemote) Stat(ctx context.Context, key string) (string, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statMiss > 0 {
		f.statMiss--
		return "", 0, &apiFault{Status: 404, Code: "not_found", Body: "not found"}
	}
	if !f.statFound {
		return "", 0, &apiFault{Status: 404, Code: "not_found", Body: "not found"}
	}
	return f.statETag, f.statSize, nil
}

func (f *fakeRemote) Abort(ctx context.Context, key, uploadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abortTries = append(f.abortTries, uploadID)
	if f.partsLive > 0 {
		f.earlyAborts++
		return &apiFault{Status: 400, Code: "request.invalid", Body: "unlinkat"}
	}
	if f.abortFailLeft > 0 {
		f.abortFailLeft--
		return &apiFault{Status: 400, Code: "request.invalid", Body: "unlinkat"}
	}
	f.aborts = append(f.aborts, uploadID)
	return nil
}

func (f *fakeRemote) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openOffsets = append(f.openOffsets, offset)
	if f.openErr != nil {
		return nil, "", 0, 0, f.openErr
	}
	if f.openStatus >= 400 && f.openFails != 0 {
		if f.openFails > 0 {
			f.openFails--
		}
		status := f.openStatus
		code := "internal"
		if status == http.StatusForbidden {
			code = "access.forbidden"
		}
		if status == http.StatusUnauthorized {
			code = "auth.unauthorized"
		}
		return nil, "", 0, status, &apiFault{Status: status, Code: code}
	}
	if f.netLeft > 0 {
		f.netLeft--
		return nil, "", 0, 0, errNetwork
	}
	if f.noRange && offset > 0 {
		return nil, "", 0, 403, errNoRange
	}
	etag := f.forceETag
	if etag == "" {
		etag = "aaa"
	}
	data := append([]byte(nil), f.data...)
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	return io.NopCloser(bytes.NewReader(data[offset:])), etag, int64(len(data)), 200, nil
}

func (f *fakeRemote) List(ctx context.Context, prefix, startAfter string) ([]listedObject, bool, string, error) {
	return nil, false, "", nil
}

func testEngine(t *testing.T, fake *fakeRemote) *Engine {
	t.Helper()
	j, err := openJournal(filepath.Join(t.TempDir(), "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	e := newEngine(j)
	e.syncRun = true
	e.limits = Limits{Threshold: 8, PartSize: 4, Transfers: 2, PartParallel: 2}
	e.progressEvery = 0
	e.build = func(creds, string) (remote, error) { return fake, nil }
	e.sleep = func(time.Duration) {}
	e.assembleFor = 40 * time.Millisecond
	e.assembleEvery = time.Millisecond
	return e
}

func writeTemp(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUploadMissingLocalFile(t *testing.T) {
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	missing := filepath.Join(t.TempDir(), "does-not-exist.bin")
	task, err := e.Enqueue(enqueueReq{
		Profile:   Profile{ID: "p"},
		Direction: "upload",
		Bucket:    "photos",
		LocalPath: missing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "error" || task.Message != "local.file_missing" {
		t.Fatalf("state %s message %q", task.State, task.Message)
	}
	if fake.puts != 0 {
		t.Fatal("a missing file must not be uploaded")
	}
	gone := &Task{Files: []fileJob{{Local: missing, Key: "gone.bin"}}}
	err = e.uploadJob(context.Background(), gone, 0, fake)
	if !errors.Is(err, errLocalFileMissing) {
		t.Fatalf("uploadJob: %v", err)
	}
	if safeTransferMessage(err) != "local.file_missing" {
		t.Fatalf("message %q", safeTransferMessage(err))
	}
}

func TestResumeSkipsConfirmedParts(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	info, _ := os.Stat(path)
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "upload", State: "running", Bucket: "b",
		LocalPath: path, Key: "k",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1", Parts: []partRec{{Number: 1, ETag: "e1"}},
		}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "super-secret-token", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
	if fake.initiates != 0 {
		t.Fatalf("initiate %d", fake.initiates)
	}
	if len(fake.partNums) != 2 || fake.partNums[0] == 1 || fake.partNums[1] == 1 {
		t.Fatalf("parts %v", fake.partNums)
	}
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(e.journal.db.Path()), "transfers.db"))
	if bytes.Contains(raw, []byte("super-secret-token")) {
		t.Fatal("journal contains the token")
	}
}

func TestInterruptedUploadResumesAfterRestart(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	info, _ := os.Stat(path)
	dbPath := filepath.Join(t.TempDir(), "transfers.db")
	j, err := openJournal(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "upload", State: "running", Bucket: "b",
		LocalPath: path, Key: "k",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1", Parts: []partRec{{Number: 1, ETag: "e1"}},
		}},
	}
	if err := j.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = openJournal(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	got, err := j.Get("t1")
	if err != nil || got.State != "interrupted" || got.Message != "resume_after_restart" {
		t.Fatalf("after restart %+v %v", got, err)
	}
	if len(got.Files) != 1 || got.Files[0].UploadID != "up1" || len(got.Files[0].Parts) != 1 {
		t.Fatalf("journal lost the upload %+v", got.Files)
	}
	fake := &fakeRemote{}
	e := newEngine(j)
	e.syncRun = true
	e.limits = Limits{Threshold: 8, PartSize: 4, Transfers: 2, PartParallel: 2}
	e.progressEvery = 0
	e.build = func(creds, string) (remote, error) { return fake, nil }
	e.sleep = func(time.Duration) {}
	n, err := e.Resume(Profile{ID: "p"}, "", "", "")
	if err != nil || n != 0 {
		t.Fatalf("resume without credentials %d %v", n, err)
	}
	n, err = e.Resume(Profile{ID: "p"}, "super-secret-token", "", "")
	if err != nil || n != 1 {
		t.Fatalf("resume %d %v", n, err)
	}
	got, _ = e.journal.Get("t1")
	if got.State != "done" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
	if fake.initiates != 0 {
		t.Fatalf("initiate %d", fake.initiates)
	}
	if len(fake.partNums) != 2 || fake.partNums[0] == 1 || fake.partNums[1] == 1 {
		t.Fatalf("parts %v", fake.partNums)
	}
	raw, _ := os.ReadFile(dbPath)
	if bytes.Contains(raw, []byte("super-secret-token")) {
		t.Fatal("journal contains the token")
	}
}

func TestInterruptedDownloadResumesFromOffset(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "a.txt")
	dbPath := filepath.Join(t.TempDir(), "transfers.db")
	j, err := openJournal(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "download", State: "running", Bucket: "b",
		Key: "a.txt", DestPath: filepath.Dir(dest),
		Files: []fileJob{{
			Local: dest, Key: "a.txt", Size: 10, Offset: 4, ETag: "aaa",
		}},
	}
	if err := j.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest+".dspart", []byte("0123"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = openJournal(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	fake := &fakeRemote{data: []byte("0123456789"), forceETag: "aaa"}
	e := newEngine(j)
	e.syncRun = true
	e.limits = Limits{Threshold: 8, PartSize: 4, Transfers: 1, PartParallel: 1}
	e.progressEvery = 0
	e.build = func(creds, string) (remote, error) { return fake, nil }
	e.sleep = func(time.Duration) {}
	if _, err := e.Resume(Profile{ID: "p"}, "token", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := e.journal.Get("t1")
	if got.State != "done" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
	if len(fake.openOffsets) == 0 || fake.openOffsets[0] != 4 {
		t.Fatalf("offsets %v", fake.openOffsets)
	}
	body, err := os.ReadFile(dest)
	if err != nil || string(body) != "0123456789" {
		t.Fatalf("body %q %v", body, err)
	}
}

func TestFileChangeAbortsUpload(t *testing.T) {
	path := writeTemp(t, []byte("0123456789"))
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", Bucket: "b", LocalPath: path,
		Files: []fileJob{{Local: path, Key: "k", Size: 3, MtimeUnix: 1, UploadID: "old"}},
	}
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	if len(fake.aborts) == 0 || fake.aborts[0] != "old" {
		t.Fatalf("aborts %v", fake.aborts)
	}
	got, _ := e.journal.Get("t1")
	if got.Message != "file_changed" && got.State != "done" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
	if got.State != "done" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
}

func TestUploadNotFoundStartsOver(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	info, _ := os.Stat(path)
	fake := &fakeRemote{notFoundOnce: true}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path,
		Files: []fileJob{{Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(), UploadID: "gone"}},
	}
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || fake.initiates < 1 {
		t.Fatalf("state %s initiates %d parts %v", got.State, fake.initiates, fake.partNums)
	}
}

func TestCompleteETagStartsOver(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	info, _ := os.Stat(path)
	fake := &fakeRemote{etagFailOnce: true}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path,
		Files: []fileJob{{Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(), UploadID: "up"}},
	}
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || fake.completes < 2 || len(fake.aborts) == 0 {
		t.Fatalf("state %s completes %d aborts %v", got.State, fake.completes, fake.aborts)
	}
}

func TestCancelAbortsMultipart(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	fake := &fakeRemote{block: make(chan struct{})}
	e := testEngine(t, fake)
	e.syncRun = false
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path,
		Files: []fileJob{{Local: path, Key: "k", Size: 10, UploadID: ""}},
	}
	info, _ := os.Stat(path)
	task.Files[0].Size = info.Size()
	task.Files[0].MtimeUnix = info.ModTime().Unix()
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	done := make(chan struct{})
	go func() {
		e.process("t1")
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		n := len(fake.partNums)
		fake.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := e.Cancel("t1"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not finish")
	}
	got, _ := e.journal.Get("t1")
	if got.State != "cancelled" || len(fake.aborts) == 0 {
		t.Fatalf("state %s aborts %v", got.State, fake.aborts)
	}
}

func TestCancelFailedMultipartAborts(t *testing.T) {
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", State: "error", Message: "tls.untrusted", Bucket: "b", Key: "k", UploadID: "up1",
		Files: []fileJob{{Key: "k", UploadID: "up1"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	// A finished task used to keep its cancel func. Cancel treated that as
	// "still running" and returned without aborting or changing the state.
	e.live["t1"] = &liveTask{cancel: func() {}, decide: make(chan decision, 1)}
	if err := e.cancel("t1", creds{Token: "t", Profile: Profile{ID: "p"}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "cancelled" || len(fake.aborts) != 1 || fake.aborts[0] != "up1" {
		t.Fatalf("state %s aborts %v", got.State, fake.aborts)
	}
}

func TestDownloadETagMismatchRestarts(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcd"), forceETag: "bbb"}
	e := testEngine(t, fake)
	dest := t.TempDir()
	task := Task{
		ID: "t1", Direction: "download", State: "running", DestPath: dest, Key: "a.txt",
		Files: []fileJob{{Key: "a.txt", Size: 4, ETag: "aaa", Offset: 2}},
	}
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "error" || got.Message != "etag_mismatch" {
		t.Fatalf("state %s %s offsets %v", got.State, got.Message, fake.openOffsets)
	}
	if len(fake.openOffsets) < 2 || fake.openOffsets[1] != 0 {
		t.Fatalf("offsets %v", fake.openOffsets)
	}
}

func TestNetworkBackoffThenSuccess(t *testing.T) {
	path := writeTemp(t, []byte("hi"))
	fake := &fakeRemote{netLeft: 2}
	e := testEngine(t, fake)
	var sleeps []time.Duration
	e.sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	task, err := e.Enqueue(enqueueReq{Profile: Profile{ID: "p"}, Token: "super-secret-token", Direction: "upload", Bucket: "b", Key: "k", LocalPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "done" {
		t.Fatalf("state %s %s", task.State, task.Message)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 2*time.Second {
		t.Fatalf("backoff %v", sleeps)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(e.journal.db.Path()), "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("super-secret-token")) {
		t.Fatal("secret in journal")
	}
}

func TestSymlinkIsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task, err := e.Enqueue(enqueueReq{Token: "t", Direction: "upload", Bucket: "b", Key: "pre", LocalPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "done" {
		t.Fatalf("state %s %s", task.State, task.Message)
	}
	var skipped bool
	for _, note := range task.Report {
		if note.Reason == "symlink" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("report %+v keys %v", task.Report, fake.keys)
	}
	if len(fake.keys) != 1 || fake.keys[0] != "pre/a.txt" {
		t.Fatalf("keys %v", fake.keys)
	}
}

func TestPresignUnavailableRestartsDownload(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcd"), noRange: true}
	e := testEngine(t, fake)
	dest := t.TempDir()
	task := Task{
		ID: "t1", Direction: "download", State: "running", DestPath: dest, Key: "a.txt",
		Files: []fileJob{{Key: "a.txt", Size: 4, ETag: "aaa", Offset: 2}},
	}
	_ = e.journal.Save(task)
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || got.Message != "presign_unavailable" {
		t.Fatalf("state %s %s offsets %v", got.State, got.Message, fake.openOffsets)
	}
	body, err := os.ReadFile(filepath.Join(dest, "a.txt"))
	if err != nil || string(body) != "abcd" {
		t.Fatalf("body %q err %v", body, err)
	}
}

func TestSlowCompleteRetriesWithoutAbort(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	fake := &fakeRemote{completeDelay: 200 * time.Millisecond, completeFailLeft: 1}
	e := testEngine(t, fake)
	started := time.Now()
	task, err := e.Enqueue(enqueueReq{Token: "t", Direction: "upload", Bucket: "b", Key: "k", LocalPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "done" {
		t.Fatalf("state %s %s", task.State, task.Message)
	}
	if time.Since(started) < 200*time.Millisecond {
		t.Fatal("complete returned before the slow server finished")
	}
	if fake.completes < 2 || len(fake.aborts) != 0 {
		t.Fatalf("completes %d aborts %v parts %v", fake.completes, fake.aborts, fake.partNums)
	}
	if len(fake.partNums) != 3 {
		t.Fatalf("parts were sent again: %v", fake.partNums)
	}
	if task.BytesDone != 10 || task.Size != 10 {
		t.Fatalf("progress %d/%d", task.BytesDone, task.Size)
	}
}

func partMD5(body string) string {
	sum := md5.Sum([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestCompleteTimeoutUsesExistingObject(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	parts := []partRec{{Number: 1, ETag: partMD5("a")}, {Number: 2, ETag: partMD5("b")}, {Number: 3, ETag: partMD5("c")}}
	want := multipartETag(parts)
	fake := &fakeRemote{completeFailLeft: 1, statFound: true, statSize: 10, statETag: want}
	e := testEngine(t, fake)
	info, _ := os.Stat(path)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1",
			Parts:    parts,
		}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || len(fake.aborts) != 0 || fake.initiates != 0 {
		t.Fatalf("state %s %s aborts %v initiates %d", got.State, got.Message, fake.aborts, fake.initiates)
	}
	if len(got.Files) != 1 || got.Files[0].ETag != want || got.Files[0].UploadID != "" {
		t.Fatalf("file %+v", got.Files)
	}
}

func TestProgressWhileUploadIsRunning(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	fake := &fakeRemote{block: make(chan struct{})}
	e := testEngine(t, fake)
	e.syncRun = false
	info, _ := os.Stat(path)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b",
		Files: []fileJob{{Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix()}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	done := make(chan struct{})
	go func() {
		e.process("t1")
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	var mid Task
	for time.Now().Before(deadline) {
		mid, _ = e.journal.Get("t1")
		if mid.BytesDone > 0 && mid.State == "running" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if mid.BytesDone <= 0 || mid.State != "running" {
		t.Fatalf("no progress while running: %+v", mid)
	}
	close(fake.block)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not finish")
	}
	got, _ := e.journal.Get("t1")
	if got.State != "done" || got.BytesDone != 10 || got.Size != 10 {
		t.Fatalf("state %s progress %d/%d", got.State, got.BytesDone, got.Size)
	}
}

func TestDownloadReportsSizeAndProgress(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcdefghi"), statFound: true, statSize: 9, statETag: "aaa"}
	e := testEngine(t, fake)
	dest := t.TempDir()
	task, err := e.Enqueue(enqueueReq{Token: "t", Direction: "download", Bucket: "b", Key: "a.txt", DestPath: dest})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "done" || task.Size != 9 || task.BytesDone != 9 {
		t.Fatalf("state %s progress %d/%d %s", task.State, task.BytesDone, task.Size, task.Message)
	}
	body, err := os.ReadFile(filepath.Join(dest, "a.txt"))
	if err != nil || string(body) != "abcdefghi" {
		t.Fatalf("body %q err %v", body, err)
	}
}

func TestDirectoryJournalDoesNotRewriteEveryFile(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 30; i++ {
		name := filepath.Join(dir, "f"+itoa(i)+".txt")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task, err := e.Enqueue(enqueueReq{Token: "t", Direction: "upload", Bucket: "b", Key: "pre", LocalPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "done" || task.BytesDone != 30 || task.FilesTotal != 30 || task.FilesDone != 30 {
		t.Fatalf("state %s progress %d files %d/%d %s", task.State, task.BytesDone, task.FilesDone, task.FilesTotal, task.Message)
	}
	var taskBlob []byte
	var files int
	err = e.journal.db.View(func(tx *bolt.Tx) error {
		taskBlob = append([]byte(nil), tx.Bucket([]byte("tasks")).Get([]byte(task.ID))...)
		c := tx.Bucket([]byte("files")).Cursor()
		prefix := []byte(task.ID + "\x00")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			files++
			if files == 1 && !bytes.Contains(v, []byte(`"done":true`)) {
				t.Fatalf("file row not done: %s", v)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files != 30 {
		t.Fatalf("file rows %d", files)
	}
	if bytes.Contains(taskBlob, []byte("f0.txt")) || len(taskBlob) > 8192 {
		t.Fatalf("task row embedded the file list (%d bytes)", len(taskBlob))
	}
}

func TestSmallFilesOverlapAndBatchSync(t *testing.T) {
	dir := t.TempDir()
	const n = 40
	for i := 0; i < n; i++ {
		name := filepath.Join(dir, "f"+itoa(i)+".txt")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	release := make(chan struct{})
	fake := &fakeRemote{gate: release}
	e := testEngine(t, fake)
	e.limits.Transfers = 4
	e.progressEvery = time.Hour
	e.journal.flushEvery = time.Hour
	e.journal.batchSize = 32
	before := e.journal.Syncs()
	type enqueueResult struct {
		task Task
		err  error
	}
	errCh := make(chan enqueueResult, 1)
	go func() {
		task, err := e.Enqueue(enqueueReq{Token: "t", Direction: "upload", Bucket: "b", Key: "pre", LocalPath: dir})
		errCh <- enqueueResult{task: task, err: err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fake.maxPuts() < e.limits.Transfers {
		time.Sleep(5 * time.Millisecond)
	}
	if fake.maxPuts() < e.limits.Transfers {
		t.Fatalf("in flight %d, want at least %d", fake.maxPuts(), e.limits.Transfers)
	}
	close(release)
	var got Task
	select {
	case result := <-errCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
		got = result.task
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not finish")
	}
	if got.State != "done" || got.FilesDone != n || got.FilesTotal != n {
		t.Fatalf("state %s files %d/%d %s", got.State, got.FilesDone, got.FilesTotal, got.Message)
	}
	rows, err := e.journal.LoadFiles(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != n {
		t.Fatalf("file rows %d", len(rows))
	}
	for i := range rows {
		if !rows[i].Done {
			t.Fatalf("row %d not done", i)
		}
	}
	delta := e.journal.Syncs() - before
	if delta < 1 || delta >= n {
		t.Fatalf("syncs %d for %d files", delta, n)
	}
}

func TestSlowCompleteUsesUntimedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/complete") {
			time.Sleep(200 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":{"etag":"abc"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client, err := newClient(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	client.Stream.Transport.(*http.Transport).ResponseHeaderTimeout = 50 * time.Millisecond
	if client.Assemble.Transport.(*http.Transport).ResponseHeaderTimeout != 0 {
		t.Fatal("assemble client has a header timeout")
	}
	remote := &restRemote{client: client, bucket: "b"}
	etag, err := remote.Complete(context.Background(), "k", "up", []partRec{{Number: 1, ETag: "e1"}})
	if err != nil || etag != "abc" {
		t.Fatalf("etag %q err %v", etag, err)
	}
}

const signedURL = "https://storage.example/b/k?X-Amz-Credential=AKIASECRET&X-Amz-Signature=deadbeef"

func TestWrapNetTreatsHeaderTimeoutAsNetwork(t *testing.T) {
	header := &url.Error{Op: "Get", URL: signedURL, Err: context.DeadlineExceeded}
	if !errors.Is(header, context.DeadlineExceeded) {
		t.Fatal("fixture is not a header timeout")
	}
	got := wrapNet(header)
	if !errors.Is(got, errNetwork) || errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("header timeout: %v", got)
	}
	if strings.Contains(got.Error(), "AKIASECRET") || strings.Contains(got.Error(), "deadbeef") || strings.Contains(got.Error(), "X-Amz-") {
		t.Fatalf("credential in error: %s", got)
	}
	if !strings.Contains(got.Error(), "storage.example/b/k") {
		t.Fatalf("host was dropped: %s", got)
	}
	reset := wrapNet(&url.Error{Op: "Get", URL: signedURL, Err: errors.New("read tcp 127.0.0.1:1->127.0.0.1:2: connection reset by peer")})
	if !errors.Is(reset, errNetwork) || strings.Contains(reset.Error(), "X-Amz-Signature") {
		t.Fatalf("reset: %v", reset)
	}
	ioTimeout := wrapNet(&url.Error{Op: "Get", URL: signedURL, Err: errors.New("i/o timeout")})
	if !errors.Is(ioTimeout, errNetwork) || errors.Is(ioTimeout, context.DeadlineExceeded) || strings.Contains(ioTimeout.Error(), "deadbeef") {
		t.Fatalf("i/o timeout: %v", ioTimeout)
	}
	canceled := wrapNet(&url.Error{Op: "Get", URL: signedURL, Err: context.Canceled})
	if !errors.Is(canceled, context.Canceled) || errors.Is(canceled, errNetwork) || strings.Contains(canceled.Error(), "http") {
		t.Fatalf("cancel: %v", canceled)
	}
	tlsErr := wrapNet(&url.Error{Op: "Get", URL: "https://storage.example/b/k", Err: errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")})
	if errors.Is(tlsErr, errNetwork) || errors.Is(tlsErr, context.DeadlineExceeded) || tlsErr.Error() != "tls.untrusted" {
		t.Fatalf("tls should fail immediately: %v", tlsErr)
	}
	if strings.Contains(tlsErr.Error(), "storage.example") || strings.Contains(tlsErr.Error(), "/b/k") {
		t.Fatalf("tls path leaked: %v", tlsErr)
	}
	if retryableTransfer(&apiFault{Status: http.StatusUnauthorized, Code: "auth.unauthorized"}) ||
		retryableTransfer(&apiFault{Status: http.StatusForbidden, Code: "access.forbidden"}) ||
		retryableTransfer(&apiFault{Status: http.StatusBadRequest, Code: "request.invalid"}) ||
		retryableTransfer(tlsErr) {
		t.Fatal("permanent error was treated as a network wait")
	}
	if !retryableTransfer(&apiFault{Status: http.StatusInternalServerError, Code: "internal"}) ||
		!retryableTransfer(&apiFault{Status: http.StatusTooManyRequests, Code: "rate_limited"}) ||
		!retryableTransfer(got) {
		t.Fatal("retryable error was treated as fatal")
	}
}

func TestStalledHeadersWaitAndResume(t *testing.T) {
	payload := []byte("0123456789")
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/objects/") {
			http.NotFound(w, r)
			return
		}
		if gets.Add(1) == 1 {
			select {
			case <-r.Context().Done():
			case <-time.After(200 * time.Millisecond):
			}
			return
		}
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	client, err := newClient(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	client.Stream.Transport.(*http.Transport).ResponseHeaderTimeout = 50 * time.Millisecond
	j, err := openJournal(filepath.Join(t.TempDir(), "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	e := newEngine(j)
	e.syncRun = true
	e.sleep = func(time.Duration) {}
	e.build = func(c creds, bucket string) (remote, error) {
		return &restRemote{client: client, bucket: bucket}, nil
	}
	dest := t.TempDir()
	task := Task{
		ID: "t1", Direction: "download", State: "running", Bucket: "b", Key: "k", DestPath: dest,
		Files: []fileJob{{Key: "k", Size: int64(len(payload)), ETag: "abc"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "done" || gets.Load() < 2 {
		t.Fatalf("state %s %s gets %d", got.State, got.Message, gets.Load())
	}
	body, err := os.ReadFile(filepath.Join(dest, "k"))
	if err != nil || !bytes.Equal(body, payload) {
		t.Fatalf("body %q err %v", body, err)
	}
}

func TestJournalScrubsPresignedURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transfers.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := `Get "` + signedURL + `": net/http: timeout awaiting response headers`
	task := Task{ID: "t1", State: "error", Message: msg, Report: []PathNote{{Reason: msg}}}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("tasks"))
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket([]byte("files")); err != nil {
			return err
		}
		return b.Put([]byte("t1"), raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	got, err := j.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Message, "X-Amz-") || strings.Contains(got.Message, "AKIASECRET") || strings.Contains(got.Message, "deadbeef") {
		t.Fatalf("message %s", got.Message)
	}
	if len(got.Report) != 1 || strings.Contains(got.Report[0].Reason, "X-Amz-Signature") {
		t.Fatalf("report %+v", got.Report)
	}
	var stored []byte
	err = j.db.View(func(tx *bolt.Tx) error {
		stored = append([]byte(nil), tx.Bucket([]byte("tasks")).Get([]byte("t1"))...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("X-Amz-Credential")) || bytes.Contains(stored, []byte("deadbeef")) {
		t.Fatalf("row %s", stored)
	}
	shown := safeTransferMessage(&url.Error{Op: "Get", URL: signedURL, Err: errors.New("broken pipe")})
	if strings.Contains(shown, "deadbeef") || strings.Contains(shown, "X-Amz-Credential") {
		t.Fatalf("list text %s", shown)
	}
}

func TestRetriedCompleteRequiresMultipartETag(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	info, _ := os.Stat(path)
	parts := []partRec{{Number: 1, ETag: partMD5("a")}, {Number: 2, ETag: partMD5("b")}, {Number: 3, ETag: partMD5("c")}}
	want := multipartETag(parts)
	fake := &fakeRemote{completeFailLeft: 1, completeStatus: 400, statMiss: 2, statFound: true, statSize: 10, statETag: `"` + want + `"`}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1", Parts: parts,
		}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || len(fake.aborts) != 0 || fake.completes != 1 {
		t.Fatalf("state %s %s completes %d aborts %v", got.State, got.Message, fake.completes, fake.aborts)
	}
	if len(got.Files) != 1 || got.Files[0].ETag != want || got.Files[0].UploadID != "" {
		t.Fatalf("file %+v", got.Files)
	}

	other := &fakeRemote{completeFailLeft: 1, completeStatus: 400, statFound: true, statSize: 10, statETag: "old-object"}
	e2 := testEngine(t, other)
	task.ID = "t2"
	task.State = "running"
	task.Files[0].UploadID = "up1"
	task.Files[0].ETag = ""
	if err := e2.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e2.live["t2"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e2.process("t2")
	miss, _ := e2.journal.Get("t2")
	if miss.State == "done" || len(other.aborts) != 0 {
		t.Fatalf("same-size object was accepted: %+v aborts %v", miss, other.aborts)
	}
	if len(miss.Files) != 1 || miss.Files[0].UploadID == "" {
		t.Fatalf("upload id cleared: %+v", miss.Files)
	}
}

func TestRetryContinuesFailedDownload(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcdefghij")}
	e := testEngine(t, fake)
	dest := t.TempDir()
	local := filepath.Join(dest, "a.txt")
	if err := os.WriteFile(local+".dspart", []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "download", State: "error", Bucket: "b", Key: "a.txt", DestPath: dest,
		Message: `Get "` + signedURL + `": net/http: timeout awaiting response headers`,
		Files:   []fileJob{{Key: "a.txt", Local: local, Size: 10, ETag: "aaa", Offset: 4}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	saved, _ := e.journal.Get("t1")
	if strings.Contains(saved.Message, "AKIASECRET") || strings.Contains(saved.Message, "deadbeef") {
		t.Fatalf("saved %s", saved.Message)
	}
	if err := e.Retry(Profile{ID: "p"}, "t1", "token", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := e.journal.Get("t1")
	if got.State != "done" || len(fake.openOffsets) == 0 || fake.openOffsets[0] != 4 {
		t.Fatalf("state %s %s offsets %v", got.State, got.Message, fake.openOffsets)
	}
	body, err := os.ReadFile(local)
	if err != nil || string(body) != "abcdefghij" {
		t.Fatalf("body %q err %v", body, err)
	}
	if strings.Contains(got.Message, "X-Amz-") {
		t.Fatalf("message %s", got.Message)
	}
}

func TestRetriedCompleteAcceptsFileMD5(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 10)
	path := writeTemp(t, body)
	sum := md5.Sum(body)
	fileETag := hex.EncodeToString(sum[:])
	info, _ := os.Stat(path)
	parts := []partRec{{Number: 1, ETag: partMD5("a")}, {Number: 2, ETag: partMD5("b")}, {Number: 3, ETag: partMD5("c")}}
	if multipartETag(parts) == fileETag {
		t.Fatal("fixture etags collide")
	}
	fake := &fakeRemote{completeFailLeft: 1, completeStatus: http.StatusBadRequest, statFound: true, statSize: 10, statETag: fileETag}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b", Message: "upload_not_found",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1", Parts: parts,
		}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || got.Message != "" || len(fake.aborts) != 0 || fake.completes != 1 || fake.initiates != 0 {
		t.Fatalf("state %s %q completes %d aborts %v initiates %d", got.State, got.Message, fake.completes, fake.aborts, fake.initiates)
	}
	if len(got.Files) != 1 || got.Files[0].ETag != fileETag || got.Files[0].ContentMD5 != fileETag || got.Files[0].UploadID != "" {
		t.Fatalf("file %+v", got.Files)
	}

	missing := &fakeRemote{completeFailLeft: 1, completeStatus: http.StatusNotFound, statFound: true, statSize: 10, statETag: fileETag}
	e2 := testEngine(t, missing)
	task.ID = "t2"
	task.State = "running"
	task.Message = "upload_not_found"
	task.Files[0].UploadID = "up1"
	task.Files[0].ETag = ""
	task.Files[0].ContentMD5 = ""
	task.Files[0].Done = false
	if err := e2.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e2.live["t2"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e2.process("t2")
	again, _ := e2.journal.Get("t2")
	if again.State != "done" || again.Message != "" || len(missing.aborts) != 0 || missing.initiates != 0 {
		t.Fatalf("404 re-uploaded: state %s %q aborts %v initiates %d parts %v", again.State, again.Message, missing.aborts, missing.initiates, missing.partNums)
	}
	if len(again.Files) != 1 || again.Files[0].ETag != fileETag {
		t.Fatalf("file %+v", again.Files)
	}
}

func TestForbiddenDownloadFailsWithoutRetry(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcdefghij"), openStatus: http.StatusForbidden, openFails: 5}
	e := testEngine(t, fake)
	dest := t.TempDir()
	local := filepath.Join(dest, "a.txt")
	task := Task{
		ID: "t1", Direction: "download", State: "running", DestPath: dest,
		Files: []fileJob{{Key: "a.txt", Local: local, Size: 10, ETag: "aaa", Offset: 4}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local+".dspart", []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "error" || len(fake.openOffsets) != 1 || fake.openOffsets[0] != 4 {
		t.Fatalf("state %s %s offsets %v", got.State, got.Message, fake.openOffsets)
	}
	if _, err := os.Stat(local + ".dspart"); err != nil {
		t.Fatal("partial removed on a permanent error")
	}
}

func TestServerErrorKeepsDownloadOffset(t *testing.T) {
	fake := &fakeRemote{data: []byte("abcdefghij"), openStatus: http.StatusInternalServerError, openFails: 1}
	e := testEngine(t, fake)
	dest := t.TempDir()
	local := filepath.Join(dest, "a.txt")
	if err := os.WriteFile(local+".dspart", []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", Direction: "download", State: "running", DestPath: dest, Message: "waiting_network",
		Files: []fileJob{{Key: "a.txt", Local: local, Size: 10, ETag: "aaa", Offset: 4}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || got.Message != "" || len(fake.openOffsets) != 2 || fake.openOffsets[0] != 4 || fake.openOffsets[1] != 4 {
		t.Fatalf("state %s %q offsets %v", got.State, got.Message, fake.openOffsets)
	}
	body, err := os.ReadFile(local)
	if err != nil || string(body) != "abcdefghij" {
		t.Fatalf("body %q err %v", body, err)
	}
}

func TestPresignedServerErrorKeepsOffset(t *testing.T) {
	payload := []byte("abcdefghij")
	var signedGets atomic.Int32
	var restGets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/presign":
			_, _ = io.WriteString(w, `{"url":"`+srvURLPlaceholder(r)+`/signed"}`)
		case r.URL.Path == "/signed":
			if signedGets.Add(1) == 1 {
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			if r.Header.Get("Range") != "bytes=4-" {
				http.Error(w, "range", http.StatusBadRequest)
				return
			}
			w.Header().Set("ETag", `"aaa"`)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[4:])
		default:
			restGets.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client, err := newClient(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	j, err := openJournal(filepath.Join(t.TempDir(), "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	e := newEngine(j)
	e.syncRun = true
	e.sleep = func(time.Duration) {}
	e.build = func(c creds, bucket string) (remote, error) {
		return &restRemote{client: client, bucket: bucket}, nil
	}
	dest := t.TempDir()
	local := filepath.Join(dest, "a.txt")
	if err := os.WriteFile(local+".dspart", payload[:4], 0o644); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", Direction: "download", State: "running", Bucket: "b", Key: "a.txt", DestPath: dest,
		Files: []fileJob{{Key: "a.txt", Local: local, Size: int64(len(payload)), ETag: "aaa", Offset: 4}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, _ := e.journal.Get("t1")
	if got.State != "done" || signedGets.Load() < 2 || restGets.Load() != 0 {
		t.Fatalf("state %s %s signed %d rest %d", got.State, got.Message, signedGets.Load(), restGets.Load())
	}
	body, err := os.ReadFile(local)
	if err != nil || !bytes.Equal(body, payload) {
		t.Fatalf("body %q err %v", body, err)
	}
}

func srvURLPlaceholder(r *http.Request) string {
	return "http://" + r.Host
}

func TestCancelRemovesPartialDownload(t *testing.T) {
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	dest := t.TempDir()
	local := filepath.Join(dest, "a.txt")
	if err := os.WriteFile(local+".dspart", []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", Direction: "download", State: "waiting_network", Message: "waiting_network", DestPath: dest,
		Files: []fileJob{{Key: "a.txt", Local: local, Offset: 4}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	loaded, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	e.finishCancel(context.Background(), &loaded, fake)
	if _, err := os.Stat(local + ".dspart"); !os.IsNotExist(err) {
		t.Fatalf("partial still present: %v", err)
	}
	got, _ := e.journal.Get("t1")
	if got.State != "cancelled" || got.Message == "waiting_network" {
		t.Fatalf("state %s %s", got.State, got.Message)
	}
}

func TestTLSFailureStoresShortCode(t *testing.T) {
	const uploadID = "upload-deadbeef"
	raw := &url.Error{
		Op:  "Put",
		URL: "https://127.0.0.1:19443/api/v1/buckets/e07/multipart/" + uploadID + "/parts/36",
		Err: errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
	}
	wrapped := wrapNet(raw)
	if wrapped.Error() != "tls.untrusted" || safeTransferMessage(wrapped) != "tls.untrusted" {
		t.Fatalf("code %v", wrapped)
	}
	if strings.Contains(wrapped.Error(), uploadID) || strings.Contains(wrapped.Error(), "/parts/") {
		t.Fatalf("path in task text: %v", wrapped)
	}
	var tlsFail *tlsError
	if !errors.As(wrapped, &tlsFail) || tlsFail == nil {
		t.Fatalf("type %T", wrapped)
	}
	if strings.Contains(tlsFail.detail, uploadID) || strings.Contains(tlsFail.detail, "/parts/") || !strings.Contains(tlsFail.detail, "unknown authority") {
		t.Fatalf("detail %q", tlsFail.detail)
	}
	if netReason(wrapped) != "certificate" || retryableTransfer(wrapped) {
		t.Fatalf("reason %s retryable %v", netReason(wrapped), retryableTransfer(wrapped))
	}
	host := wrapNet(&url.Error{Op: "Get", URL: "https://storage.example/b/" + uploadID, Err: errors.New("tls: failed to verify certificate: x509: certificate is valid for example.com, not storage.example")})
	if host.Error() != "tls.failed" || strings.Contains(host.Error(), uploadID) || strings.Contains(host.Error(), "storage.example") {
		t.Fatalf("host mismatch: %v", host)
	}

	fake := &fakeRemote{data: []byte("abcdefghij"), openErr: wrapped}
	e := testEngine(t, fake)
	logDir := t.TempDir()
	logger, err := openLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	e.log = logger
	dest := t.TempDir()
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "download", State: "running", Bucket: "b", Key: "k", DestPath: dest,
		Files: []fileJob{{Key: "k", Size: 10, ETag: "abc"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	e.process("t1")
	got, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "error" || got.Message != "tls.untrusted" {
		t.Fatalf("state %s message %s", got.State, got.Message)
	}
	if strings.Contains(got.Message, uploadID) || strings.Contains(got.Message, "http") {
		t.Fatalf("stored %s", got.Message)
	}
	logger.Sync()
	body, err := os.ReadFile(filepath.Join(logDir, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("unknown authority")) || bytes.Contains(body, []byte(uploadID)) || bytes.Contains(body, []byte("/parts/36")) {
		t.Fatalf("log %s", body)
	}
}

func TestTransferListKeepsOneProfile(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	tasks := []Task{
		{ID: "a", ProfileID: "p1", State: "running", Bucket: "b", Key: "from-p1"},
		{ID: "b", ProfileID: "p2", State: "running", Bucket: "b", Key: "from-p2"},
		{ID: "c", ProfileID: "", State: "interrupted", Bucket: "b", Key: "legacy"},
	}
	for _, task := range tasks {
		if err := e.journal.Save(task); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{transfers: e}
	one, err := app.transferList(map[string]any{"id": "p1"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := one.(map[string]any)["transfers"].([]Task)
	got := map[string]bool{}
	for _, row := range rows {
		got[row.ID] = true
		if row.Key == "from-p2" {
			t.Fatalf("other profile returned: %+v", row)
		}
	}
	if !got["a"] || !got["c"] || got["b"] {
		t.Fatalf("ids %v", got)
	}
	all, err := app.transferList(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	every, _ := all.(map[string]any)["transfers"].([]Task)
	if len(every) != 3 {
		t.Fatalf("unscoped list %d", len(every))
	}
}

func TestAbortOncePerUpload(t *testing.T) {
	fake := &fakeRemote{}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", State: "error", Bucket: "b", Key: "folder", UploadID: "up1",
		Files: []fileJob{{Key: "folder/file", UploadID: "up1"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{decide: make(chan decision, 1)}
	if err := e.cancel("t1", creds{Token: "t", Profile: Profile{ID: "p"}}); err != nil {
		t.Fatal(err)
	}
	if len(fake.aborts) != 1 || fake.aborts[0] != "up1" {
		t.Fatalf("aborts %v", fake.aborts)
	}
}

func TestPauseProfileLeavesUpload(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	keep := Task{
		ID: "mine", ProfileID: "p", State: "running", Bucket: "b", Key: "k", UploadID: "up1",
		Files: []fileJob{{Key: "k", UploadID: "up1", Local: "local"}},
	}
	other := Task{ID: "theirs", ProfileID: "q", State: "running", Bucket: "b", Key: "other", UploadID: "up2"}
	if err := e.journal.Save(keep); err != nil {
		t.Fatal(err)
	}
	if err := e.journal.Save(other); err != nil {
		t.Fatal(err)
	}
	if err := e.PauseProfile("p"); err != nil {
		t.Fatal(err)
	}
	got, err := e.journal.Get("mine")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "interrupted" || got.Message != "paused_signed_out" || got.UploadID != "up1" {
		t.Fatalf("paused %+v", got)
	}
	if len(got.Files) != 1 || got.Files[0].UploadID != "up1" {
		t.Fatalf("files %+v", got.Files)
	}
	left, err := e.journal.Get("theirs")
	if err != nil || left.State != "running" {
		t.Fatalf("other profile %+v %v", left, err)
	}
}

func TestPauseProfileDoesNotAbort(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("a"), 20))
	block := make(chan struct{})
	fake := &fakeRemote{block: block}
	e := testEngine(t, fake)
	e.syncRun = false
	task, err := e.Enqueue(enqueueReq{
		Profile: Profile{ID: "p"}, Token: "tok", Direction: "upload", Bucket: "b", Key: "k", LocalPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		e.process(task.ID)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		fake.mu.Lock()
		live := fake.partsLive
		fake.mu.Unlock()
		if live > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload did not read a part")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.PauseProfile("p"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pause did not stop the upload")
	}
	got, err := e.journal.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "interrupted" || got.Message != "paused_signed_out" || got.UploadID == "" {
		t.Fatalf("state %s %s upload %s", got.State, got.Message, got.UploadID)
	}
	if got.BytesDone <= 0 {
		t.Fatalf("paused progress %d", got.BytesDone)
	}
	for _, job := range got.Files {
		for _, part := range job.Parts {
			if part.ETag != "" {
				t.Fatalf("part finished during pause: %+v", part)
			}
		}
	}
	fake.mu.Lock()
	n := len(fake.aborts)
	fake.mu.Unlock()
	if n != 0 {
		t.Fatalf("aborts %d", n)
	}
}

func TestPauseAllLeavesEveryProfile(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	one := Task{ID: "a", ProfileID: "p", State: "running", Bucket: "b", Key: "k", UploadID: "up1"}
	two := Task{ID: "b", ProfileID: "q", State: "queued", Bucket: "b", Key: "other", UploadID: "up2"}
	if err := e.journal.Save(one); err != nil {
		t.Fatal(err)
	}
	if err := e.journal.Save(two); err != nil {
		t.Fatal(err)
	}
	if err := e.PauseAll(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		got, err := e.journal.Get(id)
		if err != nil || got.State != "interrupted" || got.Message != "paused_user" || got.UploadID == "" {
			t.Fatalf("%s %+v %v", id, got, err)
		}
	}
}

func TestSignInLeavesAUserPause(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	e.syncRun = false
	task := Task{ID: "a", ProfileID: "p", State: "running", Bucket: "b", Key: "k", UploadID: "up1"}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := e.PauseAll(); err != nil {
		t.Fatal(err)
	}
	n, err := e.Resume(Profile{ID: "p"}, "tok", "", "")
	if err != nil || n != 0 {
		t.Fatalf("sign-in resumed %d %v", n, err)
	}
	got, err := e.journal.Get("a")
	if err != nil || got.State != "interrupted" || got.Message != "paused_user" || got.UploadID != "up1" {
		t.Fatalf("after sign-in %+v %v", got, err)
	}
	n, err = e.ResumeUser(Profile{ID: "p"}, "a", "tok", "", "")
	if err != nil || n != 1 {
		t.Fatalf("resume %d %v", n, err)
	}
	got, err = e.journal.Get("a")
	if err != nil || got.State != "queued" || got.Message != "" {
		t.Fatalf("queued %+v %v", got, err)
	}
}

func TestSyncProgressKeepsADisplayFloor(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	task := Task{ID: "t1", ProfileID: "p", State: "running", BytesDone: 12, Files: []fileJob{{Key: "k", Size: 40}}}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	loaded, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	e.syncProgress(&loaded)
	if loaded.BytesDone != 0 || loaded.BytesSeen != 12 {
		t.Fatalf("floor done %d seen %d", loaded.BytesDone, loaded.BytesSeen)
	}
	app := &App{transfers: e}
	raw, err := app.transferList(map[string]any{"id": "p"})
	if err != nil {
		t.Fatal(err)
	}
	items := raw.(map[string]any)["transfers"].([]Task)
	if len(items) != 1 || items[0].BytesDone != 12 || items[0].BytesSeen != 0 {
		t.Fatalf("listed %+v", items)
	}
	stored, err := e.journal.Get("t1")
	if err != nil || stored.BytesDone != 0 || stored.BytesSeen != 12 {
		t.Fatalf("journal %+v %v", stored, err)
	}
	e.addProgress(&loaded, 0, 12)
	if loaded.BytesDone != 12 || loaded.BytesSeen != 0 {
		t.Fatalf("caught up done %d seen %d", loaded.BytesDone, loaded.BytesSeen)
	}
}

func TestClearFinishedKeepsAPendingAbort(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	rows := []Task{
		{ID: "done", ProfileID: "p", State: "done", Key: "a"},
		{ID: "gone", ProfileID: "p", State: "cancelled", Key: "b"},
		{ID: "wait", ProfileID: "p", State: "cancelled", Key: "c", PendingAborts: []pendingAbort{{UploadID: "up"}}},
		{ID: "run", ProfileID: "p", State: "running", Key: "d"},
		{ID: "other", ProfileID: "q", State: "done", Key: "e"},
	}
	for _, task := range rows {
		if err := e.journal.Save(task); err != nil {
			t.Fatal(err)
		}
	}
	n, err := e.ClearFinished("p")
	if err != nil || n != 2 {
		t.Fatalf("cleared %d %v", n, err)
	}
	for _, id := range []string{"done", "gone"} {
		if _, err := e.journal.Get(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s %v", id, err)
		}
	}
	if got, err := e.journal.Get("wait"); err != nil || len(got.PendingAborts) != 1 {
		t.Fatalf("pending %+v %v", got, err)
	}
	if got, err := e.journal.Get("run"); err != nil || got.State != "running" {
		t.Fatalf("running %+v %v", got, err)
	}
	if got, err := e.journal.Get("other"); err != nil || got.State != "done" {
		t.Fatalf("other profile %+v %v", got, err)
	}
}

func TestCanceledReadStaysAndANetworkErrorDoesNot(t *testing.T) {
	e := testEngine(t, &fakeRemote{})
	task := Task{ID: "t1", State: "running", Files: []fileJob{{Key: "k", Size: 20}}}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	loaded, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	e.addProgress(&loaded, 0, 12)
	e.rollbackRead(&loaded, 0, 12, context.Canceled)
	if loaded.BytesDone != 12 {
		t.Fatalf("cancel kept %d", loaded.BytesDone)
	}
	got, err := e.journal.Get("t1")
	if err != nil || got.BytesDone != 12 {
		t.Fatalf("journal %d %v", got.BytesDone, err)
	}
	e.rollbackRead(&loaded, 0, 12, errNetwork)
	if loaded.BytesDone != 0 {
		t.Fatalf("network kept %d", loaded.BytesDone)
	}
}

func TestLogoutPausesTransfers(t *testing.T) {
	dir := t.TempDir()
	app := &App{dataDir: dir}
	journal, err := openJournal(filepath.Join(dir, "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	app.transfers = newEngine(journal)
	profile, err := app.putProfile(Profile{Name: "One", ServerURL: "http://127.0.0.1:9", AuthMethod: "password"})
	if err != nil {
		t.Fatal(err)
	}
	task := Task{ID: "t1", ProfileID: profile.ID, State: "queued", Bucket: "b", Key: "k", UploadID: "up1"}
	if err := journal.Save(task); err != nil {
		t.Fatal(err)
	}
	if _, err := app.logout(profile, ""); err != nil {
		t.Fatal(err)
	}
	got, err := journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "interrupted" || got.Message != "paused_signed_out" || got.UploadID != "up1" {
		t.Fatalf("%+v", got)
	}
}

func TestAbortRetriesUntilGone(t *testing.T) {
	fake := &fakeRemote{abortFailLeft: 2}
	e := testEngine(t, fake)
	task := Task{
		ID: "t1", State: "error", Bucket: "b", Key: "k", UploadID: "up1",
		Files: []fileJob{{Key: "k", UploadID: "up1"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := e.cancel("t1", creds{Token: "t", Profile: Profile{ID: "p"}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "cancelled" || len(got.PendingAborts) != 0 {
		t.Fatalf("state %s pending %v", got.State, got.PendingAborts)
	}
	if len(fake.aborts) != 1 || fake.aborts[0] != "up1" || len(fake.abortTries) != 3 {
		t.Fatalf("success %v tries %v", fake.aborts, fake.abortTries)
	}
	e.abortAll(context.Background(), &got, fake)
	if len(fake.aborts) != 1 || len(fake.abortTries) != 3 {
		t.Fatalf("second delete success %v tries %v", fake.aborts, fake.abortTries)
	}
}

func TestCancelWaitsForPartWorkers(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 20))
	hold := make(chan struct{})
	fake := &fakeRemote{block: make(chan struct{}), partHold: hold}
	e := testEngine(t, fake)
	e.syncRun = false
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b", Key: "k",
		Files: []fileJob{{Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix()}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", decide: make(chan decision, 1)}
	done := make(chan struct{})
	go func() {
		e.process("t1")
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		n := len(fake.partNums)
		fake.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	go func() {
		wait := time.Now().Add(2 * time.Second)
		for time.Now().Before(wait) {
			fake.mu.Lock()
			ready := fake.holding
			fake.mu.Unlock()
			if ready {
				close(hold)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(hold)
	}()
	if err := e.Cancel("t1"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not finish")
	}
	fake.mu.Lock()
	early := fake.earlyAborts
	success := append([]string(nil), fake.aborts...)
	fake.mu.Unlock()
	if early != 0 {
		t.Fatalf("abort ran while a part worker was still in flight: %d", early)
	}
	if len(success) != 1 {
		t.Fatalf("aborts %v", success)
	}
	got, err := e.journal.Get("t1")
	if err != nil || got.State != "cancelled" || len(got.PendingAborts) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPendingAbortSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transfers.db")
	journal, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", ProfileID: "p", State: "cancelled", Bucket: "b", Key: "k", UploadID: "up1",
		PendingAborts: []pendingAbort{{Key: "k", UploadID: "up1"}},
	}
	if err := journal.Save(task); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	fake := &fakeRemote{}
	e := newEngine(reopened)
	e.sleep = func(time.Duration) {}
	e.build = func(creds, string) (remote, error) { return fake, nil }
	e.retryPendingAborts(Profile{ID: "p"}, "tok", "", "")
	if len(fake.aborts) != 1 || fake.aborts[0] != "up1" {
		t.Fatalf("aborts %v", fake.aborts)
	}
	got, err := reopened.Get("t1")
	if err != nil || len(got.PendingAborts) != 0 || got.State != "cancelled" {
		t.Fatalf("%+v %v", got, err)
	}
	e.retryPendingAborts(Profile{ID: "p"}, "tok", "", "")
	if len(fake.aborts) != 1 {
		t.Fatalf("aborts after restart retry %v", fake.aborts)
	}
}

func TestSessionExpiryPausesUpload(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 20))
	fake := &fakeRemote{unauthorized: true}
	e := testEngine(t, fake)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID: "t1", ProfileID: "p", Direction: "upload", State: "running", Bucket: "b", Key: "k", LocalPath: path, UploadID: "up1",
		Files: []fileJob{{Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(), UploadID: "up1"}},
	}
	if err := e.journal.Save(task); err != nil {
		t.Fatal(err)
	}
	e.live["t1"] = &liveTask{token: "t", profile: Profile{ID: "p"}, decide: make(chan decision, 1)}
	var marked string
	e.onSession = func(id string) { marked = id }
	e.process("t1")
	got, err := e.journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "interrupted" || got.Message != "auth.session_expired" || got.UploadID != "up1" {
		t.Fatalf("%+v", got)
	}
	if len(fake.aborts) != 0 || marked != "p" {
		t.Fatalf("aborts %v marked %s", fake.aborts, marked)
	}
}
