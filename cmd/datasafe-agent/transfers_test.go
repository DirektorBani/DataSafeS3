package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	completeMissing  bool
	statFound        bool
	statSize         int64
	statETag         string
	gate             chan struct{}
	inflight         int
	maxInflight      int
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
	if f.netLeft > 0 {
		f.netLeft--
		f.mu.Unlock()
		return "", errNetwork
	}
	if f.notFoundOnce {
		f.notFoundOnce = false
		f.mu.Unlock()
		return "", &apiFault{Status: 404, Code: "multipart.upload_not_found", Body: "upload not found"}
	}
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
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
	var fail error
	if f.completeFailLeft > 0 {
		f.completeFailLeft--
		fail = fmt.Errorf("%w: timeout awaiting response headers", context.DeadlineExceeded)
	}
	missing := f.completeMissing
	f.mu.Unlock()
	if fail != nil {
		return "", fail
	}
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
	if !f.statFound {
		return "", 0, &apiFault{Status: 404, Code: "not_found", Body: "not found"}
	}
	return f.statETag, f.statSize, nil
}

func (f *fakeRemote) Abort(ctx context.Context, key, uploadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborts = append(f.aborts, uploadID)
	return nil
}

func (f *fakeRemote) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openOffsets = append(f.openOffsets, offset)
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

func TestCompleteTimeoutUsesExistingObject(t *testing.T) {
	path := writeTemp(t, bytes.Repeat([]byte("x"), 10))
	fake := &fakeRemote{completeFailLeft: 1, statFound: true, statSize: 10, statETag: "assembled"}
	e := testEngine(t, fake)
	info, _ := os.Stat(path)
	task := Task{
		ID: "t1", Direction: "upload", State: "running", LocalPath: path, Bucket: "b",
		Files: []fileJob{{
			Local: path, Key: "k", Size: info.Size(), MtimeUnix: info.ModTime().Unix(),
			UploadID: "up1",
			Parts:    []partRec{{Number: 1, ETag: "e1"}, {Number: 2, ETag: "e2"}, {Number: 3, ETag: "e3"}},
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
	if len(got.Files) != 1 || got.Files[0].ETag != "assembled" || got.Files[0].UploadID != "" {
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
