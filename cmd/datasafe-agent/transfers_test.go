package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeRemote struct {
	mu           sync.Mutex
	partNums     []int
	aborts       []string
	initiates    int
	completes    int
	puts         int
	netLeft      int
	notFoundOnce bool
	etagFailOnce bool
	block        chan struct{}
	data         []byte
	forceETag    string
	noRange      bool
	openOffsets  []int64
	keys         []string
}

func (f *fakeRemote) Put(ctx context.Context, key string, body io.Reader, n int64) (string, error) {
	_, _ = io.Copy(io.Discard, body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts++
	f.keys = append(f.keys, key)
	if f.netLeft > 0 {
		f.netLeft--
		return "", errNetwork
	}
	return "put", nil
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
	defer f.mu.Unlock()
	f.completes++
	if f.etagFailOnce {
		f.etagFailOnce = false
		return "", &apiFault{Status: 400, Body: "etag mismatch"}
	}
	return "done", nil
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
	e := newEngine(j)
	e.syncRun = true
	e.limits = Limits{Threshold: 8, PartSize: 4, Transfers: 2, PartParallel: 2}
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
