package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	multipartThreshold = 64 << 20
	partBytes          = 16 << 20
	maxTransfers       = 4
	maxPartParallel    = 4
)

var (
	errNetwork     = errors.New("network")
	errNoRange     = errors.New("no range")
	errETagRestart = errors.New("etag restart")
)

// Limits are the starting queue sizes from spec 4.7.1. Tests shrink them.
type Limits struct {
	Threshold    int64
	PartSize     int64
	Transfers    int
	PartParallel int
}

func defaultLimits() Limits {
	return Limits{Threshold: multipartThreshold, PartSize: partBytes, Transfers: maxTransfers, PartParallel: maxPartParallel}
}

type remote interface {
	Put(ctx context.Context, key string, body io.Reader, n int64) (string, error)
	Initiate(ctx context.Context, key string) (string, error)
	Part(ctx context.Context, key, uploadID string, part int, body io.Reader, n int64) (string, error)
	Complete(ctx context.Context, key, uploadID string, parts []partRec) (string, error)
	Abort(ctx context.Context, key, uploadID string) error
	Open(ctx context.Context, key string, offset int64) (io.ReadCloser, string, int64, int, error)
	List(ctx context.Context, prefix, startAfter string) ([]listedObject, bool, string, error)
	Stat(ctx context.Context, key string) (string, int64, error)
}

type decision struct {
	choice string
	all    bool
}

type liveTask struct {
	profile   Profile
	token     string
	accessKey string
	secret    string
	cancel    context.CancelFunc
	busy      bool
	decide    chan decision
}

type creds struct {
	Profile   Profile
	Token     string
	AccessKey string
	Secret    string
}

// Engine runs transfers for the life of the agent process. Confirmed parts
// are in the journal. The token and the access-key secret stay in memory.
type Engine struct {
	journal       *Journal
	limits        Limits
	mu            sync.Mutex
	live          map[string]*liveTask
	wake          chan struct{}
	syncRun       bool
	sleep         func(time.Duration)
	build         func(creds, string) (remote, error)
	osName        OSName
	longPaths     bool
	progressEvery time.Duration
	progressAt    time.Time
	log           *Logger
	// assembleFor is how long a retried complete polls object-meta before
	// giving up. Zero means 30s. Tests set a short budget: their sleep is a
	// no-op, so a full 30s would spin on the wall clock.
	assembleFor   time.Duration
	assembleEvery time.Duration
}

func newEngine(j *Journal) *Engine {
	e := &Engine{
		journal:       j,
		limits:        defaultLimits(),
		live:          map[string]*liveTask{},
		wake:          make(chan struct{}, 1),
		osName:        hostOS(),
		longPaths:     true,
		progressEvery: 400 * time.Millisecond,
	}
	return e
}

func (e *Engine) start() {
	n := e.limits.Transfers
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go e.worker()
	}
}

func (e *Engine) worker() {
	for {
		id := e.take()
		if id == "" {
			select {
			case <-e.wake:
			case <-time.After(400 * time.Millisecond):
			}
			continue
		}
		e.process(id)
	}
}

func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (a *App) engine() (*Engine, error) {
	a.transferOnce.Do(func() {
		if a.transfers != nil {
			return
		}
		j, err := openJournal(filepath.Join(a.dataDir, "transfers.db"))
		if err != nil {
			a.transferErr = err
			return
		}
		eng := newEngine(j)
		eng.log = a.log
		eng.build = a.remoteFor
		eng.start()
		a.transfers = eng
	})
	if a.transferErr != nil {
		return nil, a.transferErr
	}
	return a.transfers, nil
}

type enqueueReq struct {
	Profile   Profile
	Token     string
	AccessKey string
	Secret    string
	Direction string
	Bucket    string
	Key       string
	LocalPath string
	DestPath  string
	Follow    bool
	Conflict  string
}

func (e *Engine) Enqueue(req enqueueReq) (Task, error) {
	req.Direction = strings.TrimSpace(req.Direction)
	if req.Direction != "upload" && req.Direction != "download" {
		return Task{}, fmt.Errorf("direction is required")
	}
	if strings.TrimSpace(req.Bucket) == "" {
		return Task{}, fmt.Errorf("bucket name is required")
	}
	if req.Direction == "upload" && strings.TrimSpace(req.LocalPath) == "" {
		return Task{}, fmt.Errorf("local path is required")
	}
	if req.Direction == "download" && strings.TrimSpace(req.Key) == "" {
		return Task{}, fmt.Errorf("object key is required")
	}
	if req.Direction == "download" && strings.TrimSpace(req.DestPath) == "" {
		return Task{}, fmt.Errorf("destination is required")
	}
	id, err := newID()
	if err != nil {
		return Task{}, err
	}
	now := time.Now().Unix()
	task := Task{
		ID:             id,
		ProfileID:      req.Profile.ID,
		Direction:      req.Direction,
		State:          "queued",
		LocalPath:      req.LocalPath,
		DestPath:       req.DestPath,
		Bucket:         req.Bucket,
		Key:            req.Key,
		FollowSymlinks: req.Follow,
		Conflict:       req.Conflict,
		CreatedUnix:    now,
		UpdatedUnix:    now,
	}
	if err := e.journal.Save(task); err != nil {
		return Task{}, err
	}
	e.mu.Lock()
	e.live[task.ID] = &liveTask{
		profile:   req.Profile,
		token:     req.Token,
		accessKey: req.AccessKey,
		secret:    req.Secret,
		decide:    make(chan decision, 1),
	}
	e.mu.Unlock()
	if e.syncRun {
		e.process(task.ID)
		return e.journal.Get(task.ID)
	}
	e.signal()
	return task, nil
}

func (e *Engine) List() ([]Task, error) {
	items, err := e.journal.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedUnix < items[j].CreatedUnix })
	return items, nil
}

func (e *Engine) Cancel(id string) error {
	return e.cancel(id, creds{})
}

// cancel stops a running transfer, or removes one that has already failed.
// A failed multipart upload is aborted on the server when the call still
// has the profile's credentials. A leftover cancel func from a finished
// task is not treated as "still running": that used to make Cancel a no-op
// on an error row.
func (e *Engine) cancel(id string, extra creds) error {
	e.mu.Lock()
	live := e.live[id]
	if live != nil && live.busy && live.cancel != nil {
		live.cancel()
		e.mu.Unlock()
		return nil
	}
	var remembered creds
	if live != nil {
		remembered = creds{Profile: live.profile, Token: live.token, AccessKey: live.accessKey, Secret: live.secret}
	}
	e.mu.Unlock()
	task, err := e.journal.Get(id)
	if err != nil {
		return err
	}
	if task.State == "done" || task.State == "cancelled" {
		return nil
	}
	use := extra
	if use.Token == "" && use.Secret == "" {
		use = remembered
	}
	if use.Profile.ID == "" {
		use.Profile = remembered.Profile
	}
	if use.AccessKey == "" {
		use.AccessKey = remembered.AccessKey
	}
	if e.build != nil && task.Bucket != "" && (use.Token != "" || use.Secret != "") {
		if remote, err := e.build(use, task.Bucket); err == nil && remote != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			e.abortAll(ctx, &task, remote)
			cancel()
		}
	}
	task.State = "cancelled"
	clearStale(&task)
	e.removePartial(&task)
	return e.journal.SaveMeta(task)
}

func (e *Engine) Resolve(id, choice string, all bool) error {
	switch choice {
	case "replace", "skip", "rename":
	default:
		return fmt.Errorf("choice is required")
	}
	e.mu.Lock()
	live := e.live[id]
	e.mu.Unlock()
	if live == nil {
		return fmt.Errorf("transfer is not waiting")
	}
	select {
	case live.decide <- decision{choice: choice, all: all}:
		return nil
	default:
		return fmt.Errorf("transfer is not waiting")
	}
}

// Resume continues tasks that were still running when the agent last
// exited. The journal has the upload id, confirmed parts, and download
// offset. The token and the access-key secret are supplied again by the
// signed-in profile; they are not read from the journal. A sleeping
// machine does not come through here: the process is still alive, and a
// dropped connection uses the same network retry as any other outage.
func (e *Engine) Resume(profile Profile, token, accessKey, secret string) (int, error) {
	if strings.TrimSpace(token) == "" && strings.TrimSpace(secret) == "" {
		return 0, nil
	}
	tasks, err := e.journal.List()
	if err != nil {
		return 0, err
	}
	var ids []string
	for _, task := range tasks {
		if task.State != "interrupted" || task.ProfileID != profile.ID {
			continue
		}
		task.State = "queued"
		task.Message = ""
		if err := e.journal.SaveMeta(task); err != nil {
			return len(ids), err
		}
		e.mu.Lock()
		live := e.live[task.ID]
		if live == nil {
			live = &liveTask{decide: make(chan decision, 1)}
			e.live[task.ID] = live
		}
		live.profile = profile
		live.token = token
		live.accessKey = accessKey
		live.secret = secret
		e.mu.Unlock()
		ids = append(ids, task.ID)
	}
	if e.syncRun {
		for _, id := range ids {
			e.process(id)
		}
		return len(ids), nil
	}
	if len(ids) > 0 {
		e.signal()
	}
	return len(ids), nil
}

// Retry queues one failed task again. Confirmed parts, the upload id, and
// the download offset stay. The token and the access-key secret are attached
// in memory, the same way Resume does for an interrupted task.
func (e *Engine) Retry(profile Profile, id, token, accessKey, secret string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("transfer id is required")
	}
	if strings.TrimSpace(token) == "" && strings.TrimSpace(secret) == "" {
		return fmt.Errorf("transfer is not waiting")
	}
	task, err := e.journal.Get(id)
	if err != nil {
		return fmt.Errorf("transfer was not found")
	}
	if task.State != "error" || (task.ProfileID != "" && task.ProfileID != profile.ID) {
		return fmt.Errorf("transfer is not waiting")
	}
	task.State = "queued"
	task.Message = ""
	if err := e.journal.SaveMeta(task); err != nil {
		return err
	}
	e.mu.Lock()
	live := e.live[task.ID]
	if live == nil {
		live = &liveTask{decide: make(chan decision, 1)}
		e.live[task.ID] = live
	}
	live.profile = profile
	live.token = token
	live.accessKey = accessKey
	live.secret = secret
	e.mu.Unlock()
	if e.syncRun {
		e.process(task.ID)
		return nil
	}
	e.signal()
	return nil
}

func (e *Engine) take() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	tasks, err := e.journal.List()
	if err != nil {
		return ""
	}
	for _, task := range tasks {
		if task.State != "queued" {
			continue
		}
		live := e.live[task.ID]
		if live == nil || (live.token == "" && live.secret == "") {
			continue
		}
		task.State = "running"
		if err := e.journal.Save(task); err != nil {
			return ""
		}
		return task.ID
	}
	return ""
}

func (e *Engine) process(id string) {
	task, err := e.journal.Get(id)
	if err != nil || task.State == "cancelled" || task.State == "interrupted" {
		return
	}
	e.mu.Lock()
	live := e.live[id]
	e.mu.Unlock()
	if live == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	live.busy = true
	live.cancel = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		live.busy = false
		live.cancel = nil
		live.token = ""
		live.secret = ""
		live.accessKey = ""
		e.mu.Unlock()
	}()
	if e.build == nil {
		task.State = "error"
		task.Message = "request.invalid"
		_ = e.journal.SaveMeta(task)
		return
	}
	remote, err := e.build(creds{Profile: live.profile, Token: live.token, AccessKey: live.accessKey, Secret: live.secret}, task.Bucket)
	if err != nil {
		task.State = "error"
		task.Message = "request.invalid"
		_ = e.journal.SaveMeta(task)
		return
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			e.finishCancel(ctx, &task, remote)
			return
		}
		var runErr error
		if task.Direction == "download" {
			runErr = e.runDownload(ctx, &task, remote)
		} else {
			runErr = e.runUpload(ctx, &task, remote)
		}
		if runErr == nil {
			task.State = "done"
			clearStale(&task)
			_ = e.journal.SaveMeta(task)
			return
		}
		// A user cancel, or any other cancel of this task's context, stops
		// the task. A header timeout is DeadlineExceeded inside url.Error;
		// wrapNet turns that into errNetwork. A TLS failure, a 4xx auth or
		// permission error, and an invalid request stop immediately.
		if ctx.Err() != nil || (errors.Is(runErr, context.Canceled) && !errors.Is(runErr, errNetwork)) {
			e.finishCancel(ctx, &task, remote)
			return
		}
		if retryableTransfer(runErr) {
			task.State = "waiting_network"
			task.Message = "waiting_network"
			_ = e.journal.SaveMeta(task)
			if err := e.pause(ctx, backoff); err != nil {
				e.finishCancel(ctx, &task, remote)
				return
			}
			if backoff < 60*time.Second {
				backoff *= 2
				if backoff > 60*time.Second {
					backoff = 60 * time.Second
				}
			}
			task.State = "running"
			clearStale(&task)
			_ = e.journal.SaveMeta(task)
			continue
		}
		task.State = "error"
		var tlsFail *tlsError
		if errors.As(runErr, &tlsFail) && tlsFail != nil && tlsFail.detail != "" && e.log != nil {
			e.log.Warn(tlsFail.detail)
		}
		task.Message = safeTransferMessage(runErr)
		_ = e.journal.SaveMeta(task)
		return
	}
}

func (e *Engine) pause(ctx context.Context, d time.Duration) error {
	if e.sleep != nil {
		e.sleep(d)
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (e *Engine) finishCancel(ctx context.Context, task *Task, remote remote) {
	task.State = "cancelled"
	clearStale(task)
	e.removePartial(task)
	actx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.abortAll(actx, task, remote)
	_ = e.journal.SaveMeta(*task)
	_ = ctx
}

// removePartial deletes a download's .dspart. Cancel used to leave that
// file in the destination folder. The journal no longer points at it.
func (e *Engine) removePartial(task *Task) {
	if task == nil || task.Direction != "download" {
		return
	}
	seen := map[string]struct{}{}
	for _, job := range task.Files {
		if job.Local == "" {
			continue
		}
		path := job.Local + ".dspart"
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		_ = os.Remove(path)
	}
}

// clearStale drops a status that belonged to the previous state. A task
// that is running again is not still "waiting for network", and a finished
// upload is not still "upload not found".
func clearStale(task *Task) {
	if task == nil {
		return
	}
	switch task.Message {
	case "waiting_network", "upload_not_found":
		task.Message = ""
	}
}

// retryableTransfer is a failure that can clear on its own: a dropped
// connection, a timeout, a 5xx, or a 429. Certificate errors and 4xx
// (auth, permission, invalid request) are not in this set.
func retryableTransfer(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, errNoRange) || errors.Is(err, errETagRestart) {
		return false
	}
	if errors.Is(err, errNetwork) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var fault *apiFault
	if errors.As(err, &fault) {
		return fault.Status == http.StatusTooManyRequests || fault.Status >= 500
	}
	return false
}

func (e *Engine) abortAll(ctx context.Context, task *Task, remote remote) {
	seen := map[string]string{}
	if task.UploadID != "" {
		seen[task.Key] = task.UploadID
	}
	for _, job := range task.Files {
		if job.UploadID != "" && !job.Done {
			seen[job.Key] = job.UploadID
		}
	}
	for key, id := range seen {
		_ = remote.Abort(ctx, key, id)
	}
}

func (e *Engine) save(task *Task) {
	e.mu.Lock()
	meta := *task
	e.mu.Unlock()
	_ = e.journal.SaveMeta(meta)
}

func (e *Engine) saveFile(task *Task, index int) {
	e.mu.Lock()
	e.saveFileLocked(task, index)
	e.mu.Unlock()
	e.flushProgress()
}

func (e *Engine) saveFileLocked(task *Task, index int) {
	task.FilesDone = countDone(task.Files)
	if e.journal == nil {
		return
	}
	if index >= 0 && index < len(task.Files) {
		_ = e.journal.queueFile(*task, index)
		return
	}
	_ = e.journal.SaveMeta(*task)
}

func (e *Engine) flushProgress() {
	if e.journal == nil || !e.journal.Due() {
		return
	}
	_ = e.journal.Flush()
}

func (e *Engine) storeFiles(task *Task) {
	e.mu.Lock()
	task.FilesTotal = len(task.Files)
	task.FilesDone = countDone(task.Files)
	task.Size = sumJobSizes(task.Files)
	files := append([]fileJob(nil), task.Files...)
	meta := *task
	e.mu.Unlock()
	_ = e.journal.PutFiles(meta.ID, files)
	_ = e.journal.SaveMeta(meta)
}

func (e *Engine) syncProgress(task *Task) {
	e.mu.Lock()
	task.BytesDone = confirmedBytes(task, e.limits.PartSize)
	e.mu.Unlock()
	e.save(task)
}

func (e *Engine) addProgress(task *Task, index int, n int64) {
	if n == 0 {
		return
	}
	e.mu.Lock()
	task.BytesDone += n
	if task.BytesDone < 0 {
		task.BytesDone = 0
	}
	now := time.Now()
	write := n < 0 || e.progressEvery <= 0 || now.Sub(e.progressAt) >= e.progressEvery
	if write {
		e.progressAt = now
		e.saveFileLocked(task, index)
	}
	e.mu.Unlock()
	if write {
		e.flushProgress()
	}
}

func countDone(files []fileJob) int {
	n := 0
	for i := range files {
		if files[i].Done {
			n++
		}
	}
	return n
}

func sumJobSizes(files []fileJob) int64 {
	var n int64
	for i := range files {
		if files[i].Size > 0 {
			n += files[i].Size
		}
	}
	return n
}

func confirmedBytes(task *Task, partSize int64) int64 {
	var n int64
	for i := range task.Files {
		job := &task.Files[i]
		if job.Done {
			if job.Size > 0 {
				n += job.Size
			}
			continue
		}
		if task.Direction == "download" {
			n += job.Offset
			continue
		}
		for _, part := range job.Parts {
			if part.ETag == "" {
				continue
			}
			_, sz := partBounds(job.Size, partSize, part.Number)
			n += sz
		}
	}
	return n
}

func retryableComplete(err error) bool {
	return errors.Is(err, errNetwork) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}

// maybeAssembled is a complete that may already have finished on the server.
// A timeout is one case. A 400 while the server is still concatenating parts
// is the other: the object row appears a few seconds later. A 404 is handled
// before the upload is started over, so it is not decided here.
func maybeAssembled(err error) bool {
	if retryableComplete(err) {
		return true
	}
	f, ok := err.(*apiFault)
	if !ok || faultNotFound(err) {
		return false
	}
	return f.Status == http.StatusBadRequest
}

// multipartETag is the S3 multipart ETag: MD5 of the raw part MD5s in
// part-number order, then "-" and the part count. A part ETag that is not
// 16 bytes of hex cannot be checked, and the caller must not fall back to size.
func multipartETag(parts []partRec) string {
	sorted := sortedParts(parts)
	if len(sorted) == 0 {
		return ""
	}
	raw := make([]byte, 0, len(sorted)*16)
	for _, part := range sorted {
		etag := strings.Trim(strings.TrimSpace(part.ETag), `"`)
		if etag == "" || strings.Contains(etag, "-") {
			return ""
		}
		sum, err := hex.DecodeString(etag)
		if err != nil || len(sum) != 16 {
			return ""
		}
		raw = append(raw, sum...)
	}
	sum := md5.Sum(raw)
	return hex.EncodeToString(sum[:]) + "-" + strconv.Itoa(len(sorted))
}

func sameETag(got, want string) bool {
	return strings.EqualFold(strings.Trim(got, `"`), strings.Trim(want, `"`))
}

// expectedETags is the identities a finished object may have. S3 uses the
// multipart form. Server 1.6.1 stores the MD5 of the whole file instead.
// An empty list means the client cannot tell this upload from another
// object of the same size, so the object is not accepted.
func expectedETags(parts []partRec, contentMD5 string) []string {
	var wants []string
	if multipart := multipartETag(parts); multipart != "" {
		wants = append(wants, multipart)
	}
	sum := strings.ToLower(strings.Trim(strings.TrimSpace(contentMD5), `"`))
	if len(sum) == 32 && !strings.Contains(sum, "-") {
		if _, err := hex.DecodeString(sum); err == nil {
			wants = append(wants, sum)
		}
	}
	return wants
}

func etagOneOf(got string, wants []string) bool {
	for _, want := range wants {
		if want != "" && sameETag(got, want) {
			return true
		}
	}
	return false
}

// fileMD5 streams the local file. The digest is stored on the file row
// before complete, so a restart can still recognise the object 1.6.1 built.
func fileMD5(ctx context.Context, path string) (string, error) {
	f, err := openLocal(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// waitAssembled polls object-meta until the object has the uploaded size and
// an ETag this upload could have produced. Size alone is not identity: an
// overwrite of the same length would otherwise be marked done with the old
// content.
func (e *Engine) waitAssembled(ctx context.Context, remote remote, key string, size int64, wants []string) (string, bool) {
	if len(wants) == 0 || size <= 0 {
		return "", false
	}
	budget := e.assembleFor
	if budget <= 0 {
		budget = 30 * time.Second
	}
	every := e.assembleEvery
	if every <= 0 {
		every = time.Second
	}
	deadline := time.Now().Add(budget)
	for {
		if ctx.Err() != nil {
			return "", false
		}
		etag, got, err := remote.Stat(ctx, key)
		if err == nil && got == size && etagOneOf(etag, wants) {
			return strings.Trim(etag, `"`), true
		}
		if !time.Now().Before(deadline) {
			return "", false
		}
		if err := e.pause(ctx, every); err != nil {
			return "", false
		}
	}
}

type progressReader struct {
	r  io.Reader
	fn func(int)
}

func (p progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.fn != nil {
		p.fn(n)
	}
	return n, err
}

func (e *Engine) runUpload(ctx context.Context, task *Task, remote remote) error {
	if len(task.Files) == 0 {
		info, statErr := os.Lstat(task.LocalPath)
		if statErr == nil && info.Mode().IsRegular() {
			key := task.Key
			if key == "" || strings.HasSuffix(key, "/") {
				var ok bool
				key, _, ok = UploadKey(task.Key, filepath.Base(task.LocalPath))
				if !ok {
					return fmt.Errorf("path")
				}
			}
			task.Files = []fileJob{{Local: task.LocalPath, Key: key}}
		}
	}
	if len(task.Files) == 0 {
		items, notes, err := collectUpload(task.LocalPath, task.Key, task.FollowSymlinks)
		if err != nil {
			return err
		}
		task.Report = append(task.Report, notes...)
		if len(items) == 0 {
			e.save(task)
			return nil
		}
		for _, item := range items {
			task.Files = append(task.Files, fileJob{Local: item.local, Key: item.key})
		}
	}
	if task.FilesTotal != len(task.Files) {
		e.noteUploadSizes(task)
		e.storeFiles(task)
	}
	e.syncProgress(task)
	slots := e.limits.Transfers
	if slots < 1 {
		slots = 1
	}
	sem := make(chan struct{}, slots)
	var wg sync.WaitGroup
	var first error
	var errMu sync.Mutex
	for i := range task.Files {
		if task.Files[i].Done {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			err := e.uploadJob(ctx, task, i, remote)
			if err != nil {
				errMu.Lock()
				if first == nil {
					first = err
				}
				errMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	return first
}

type uploadItem struct {
	local string
	key   string
}

func collectUpload(root, prefix string, follow bool) ([]uploadItem, []PathNote, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, nil, err
	}
	var items []uploadItem
	var notes []PathNote
	seen := map[[2]uint64]struct{}{}
	var walk func(path, rel string, depth int)
	walk = func(path, rel string, depth int) {
		if depth > 40 {
			notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "cycle"})
			return
		}
		st, err := os.Lstat(path)
		if err != nil {
			notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "unreadable"})
			return
		}
		if isLinkPath(path, st) {
			if !follow {
				notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "symlink"})
				return
			}
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "symlink"})
				return
			}
			st, err = os.Stat(target)
			if err != nil {
				notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "symlink"})
				return
			}
			if dev, ino, ok := fileID(st); ok {
				id := [2]uint64{dev, ino}
				if _, dup := seen[id]; dup {
					notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "cycle"})
					return
				}
				seen[id] = struct{}{}
			}
			path = target
		} else if dev, ino, ok := fileID(st); ok {
			seen[[2]uint64{dev, ino}] = struct{}{}
		}
		if st.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				notes = append(notes, PathNote{Action: "skip", From: rel, Reason: "unreadable"})
				return
			}
			for _, ent := range entries {
				next := ent.Name()
				childRel := next
				if rel != "" {
					childRel = rel + "/" + next
				}
				walk(filepath.Join(path, next), childRel, depth+1)
			}
			return
		}
		key, note, ok := UploadKey(prefix, rel)
		if !ok {
			notes = append(notes, note)
			return
		}
		items = append(items, uploadItem{local: path, key: key})
	}
	if info.IsDir() {
		walk(root, "", 0)
	} else {
		walk(root, filepath.Base(root), 0)
	}
	return items, notes, nil
}

func isLinkPath(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return isReparse(path, info)
}

func (e *Engine) noteUploadSizes(task *Task) {
	for i := range task.Files {
		info, err := os.Stat(task.Files[i].Local)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		task.Files[i].Size = info.Size()
		if task.Files[i].MtimeUnix == 0 {
			task.Files[i].MtimeUnix = info.ModTime().Unix()
		}
	}
}

func (e *Engine) uploadJob(ctx context.Context, task *Task, index int, remote remote) error {
	job := &task.Files[index]
	info, err := os.Stat(job.Local)
	if err != nil {
		return err
	}
	size := info.Size()
	mtime := info.ModTime().Unix()
	e.mu.Lock()
	prevSize := job.Size
	changed := job.UploadID != "" && (prevSize != size || (job.MtimeUnix != 0 && job.MtimeUnix != mtime))
	uploadID := job.UploadID
	key := job.Key
	if prevSize != size {
		task.Size += size - prevSize
	}
	if changed {
		var drop int64
		for _, part := range job.Parts {
			if part.ETag == "" {
				continue
			}
			_, sz := partBounds(prevSize, e.limits.PartSize, part.Number)
			drop += sz
		}
		task.BytesDone -= drop
		if task.BytesDone < 0 {
			task.BytesDone = 0
		}
		job.UploadID = ""
		job.Parts = nil
		job.ContentMD5 = ""
		task.UploadID = ""
		task.Parts = nil
		task.Message = "file_changed"
	}
	job.Size = size
	job.MtimeUnix = mtime
	task.MtimeUnix = mtime
	e.mu.Unlock()
	if changed {
		_ = remote.Abort(context.Background(), key, uploadID)
		e.saveFile(task, index)
	}
	if size > e.limits.Threshold {
		e.mu.Lock()
		have := job.ContentMD5
		e.mu.Unlock()
		if have == "" {
			sum, err := fileMD5(ctx, job.Local)
			if err != nil {
				return err
			}
			e.mu.Lock()
			job.ContentMD5 = sum
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
		}
	}
	if size <= e.limits.Threshold {
		f, err := openLocal(job.Local)
		if err != nil {
			return err
		}
		defer f.Close()
		var read int64
		body := progressReader{r: f, fn: func(n int) {
			read += int64(n)
			e.addProgress(task, index, int64(n))
		}}
		etag, err := remote.Put(ctx, key, body, size)
		if err != nil {
			e.addProgress(task, index, -read)
			return err
		}
		e.mu.Lock()
		job.Done = true
		job.ETag = etag
		e.saveFileLocked(task, index)
		e.mu.Unlock()
		e.flushProgress()
		return nil
	}
	restarts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := e.uploadParts(ctx, task, index, remote, size)
		if err == nil {
			e.mu.Lock()
			job.Done = true
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, errNetwork) {
			return err
		}
		if faultNotFound(err) || faultETag(err) {
			if faultNotFound(err) {
				e.mu.Lock()
				parts := append([]partRec(nil), job.Parts...)
				sum := job.ContentMD5
				e.mu.Unlock()
				if got, ok := e.waitAssembled(ctx, remote, key, size, expectedETags(parts, sum)); ok {
					e.mu.Lock()
					job.ETag = got
					job.Done = true
					job.UploadID = ""
					task.UploadID = ""
					task.Message = ""
					e.saveFileLocked(task, index)
					e.mu.Unlock()
					e.flushProgress()
					return nil
				}
			}
			e.mu.Lock()
			uploadID = job.UploadID
			if uploadID != "" {
				job.UploadID = ""
				job.Parts = nil
				task.UploadID = ""
				task.Parts = nil
			}
			if faultNotFound(err) {
				task.Message = "upload_not_found"
			} else {
				task.Message = "etag_mismatch"
			}
			task.BytesDone = confirmedBytes(task, e.limits.PartSize)
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
			if uploadID != "" {
				_ = remote.Abort(context.Background(), key, uploadID)
			}
			restarts++
			if restarts > 3 {
				return err
			}
			continue
		}
		return err
	}
}

func (e *Engine) uploadParts(ctx context.Context, task *Task, index int, remote remote, size int64) error {
	job := &task.Files[index]
	e.mu.Lock()
	uploadID := job.UploadID
	key := job.Key
	local := job.Local
	e.mu.Unlock()
	if uploadID == "" {
		id, err := remote.Initiate(ctx, key)
		if err != nil {
			return err
		}
		e.mu.Lock()
		job.UploadID = id
		job.Parts = nil
		task.UploadID = id
		uploadID = id
		e.saveFileLocked(task, index)
		e.mu.Unlock()
		e.flushProgress()
	}
	e.mu.Lock()
	total := countParts(size, e.limits.PartSize)
	missing := missingParts(job.Parts, total)
	e.mu.Unlock()
	slots := e.limits.PartParallel
	if slots < 1 {
		slots = 1
	}
	sem := make(chan struct{}, slots)
	var wg sync.WaitGroup
	var first error
	var errMu sync.Mutex
	for _, number := range missing {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(number int) {
			defer wg.Done()
			defer func() { <-sem }()
			errMu.Lock()
			stopped := first != nil
			errMu.Unlock()
			if stopped || ctx.Err() != nil {
				return
			}
			offset, n := partBounds(size, e.limits.PartSize, number)
			f, err := openLocal(local)
			if err != nil {
				errMu.Lock()
				if first == nil {
					first = err
				}
				errMu.Unlock()
				return
			}
			defer f.Close()
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				errMu.Lock()
				if first == nil {
					first = err
				}
				errMu.Unlock()
				return
			}
			var read int64
			body := progressReader{r: io.LimitReader(f, n), fn: func(dn int) {
				read += int64(dn)
				e.addProgress(task, index, int64(dn))
			}}
			etag, err := remote.Part(ctx, key, uploadID, number, body, n)
			if err != nil {
				e.addProgress(task, index, -read)
				errMu.Lock()
				if first == nil {
					first = err
				}
				errMu.Unlock()
				return
			}
			e.mu.Lock()
			job.Parts = append(job.Parts, partRec{Number: number, ETag: etag})
			task.UploadID = job.UploadID
			task.Parts = append([]partRec(nil), job.Parts...)
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
		}(number)
	}
	wg.Wait()
	if first != nil {
		return first
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	e.mu.Lock()
	parts := append([]partRec(nil), job.Parts...)
	contentMD5 := job.ContentMD5
	uploadID = job.UploadID
	e.mu.Unlock()
	etag, err := remote.Complete(ctx, key, uploadID, parts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if maybeAssembled(err) {
			if got, ok := e.waitAssembled(ctx, remote, key, size, expectedETags(parts, contentMD5)); ok {
				e.mu.Lock()
				job.ETag = got
				job.UploadID = ""
				task.UploadID = ""
				e.saveFileLocked(task, index)
				e.mu.Unlock()
				e.flushProgress()
				return nil
			}
		}
		if retryableComplete(err) {
			return errNetwork
		}
		return err
	}
	e.mu.Lock()
	job.ETag = etag
	job.UploadID = ""
	task.UploadID = ""
	e.mu.Unlock()
	return nil
}

func countParts(size, part int64) int {
	if part <= 0 {
		part = partBytes
	}
	if size <= 0 {
		return 1
	}
	n := int((size + part - 1) / part)
	if n < 1 {
		return 1
	}
	return n
}

func partBounds(size, part int64, number int) (int64, int64) {
	offset := int64(number-1) * part
	n := part
	if offset >= size {
		return offset, 0
	}
	if offset+n > size {
		n = size - offset
	}
	return offset, n
}

func missingParts(parts []partRec, total int) []int {
	have := map[int]bool{}
	for _, part := range parts {
		if part.ETag != "" {
			have[part.Number] = true
		}
	}
	var out []int
	for n := 1; n <= total; n++ {
		if !have[n] {
			out = append(out, n)
		}
	}
	return out
}

func (e *Engine) runDownload(ctx context.Context, task *Task, remote remote) error {
	if len(task.Files) == 0 {
		objects, err := e.listAll(ctx, remote, task.Key)
		if err != nil {
			return err
		}
		if len(objects) == 0 && !strings.HasSuffix(task.Key, "/") {
			objects = []listedObject{{Key: task.Key}}
		}
		for _, obj := range objects {
			task.Files = append(task.Files, fileJob{Key: obj.Key, Size: obj.Size, ETag: obj.ETag})
		}
		for i := range task.Files {
			if task.Files[i].Size > 0 {
				continue
			}
			etag, size, err := remote.Stat(ctx, task.Files[i].Key)
			if err != nil || size <= 0 {
				continue
			}
			task.Files[i].Size = size
			if task.Files[i].ETag == "" {
				task.Files[i].ETag = etag
			}
		}
		e.storeFiles(task)
	}
	e.syncProgress(task)
	used := map[string]struct{}{}
	for i := range task.Files {
		job := &task.Files[i]
		if job.Done {
			if job.Local != "" {
				used[foldName(job.Local, e.osName)] = struct{}{}
			}
			continue
		}
		if err := e.downloadJob(ctx, task, i, remote, used); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) listAll(ctx context.Context, remote remote, prefix string) ([]listedObject, error) {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		return nil, nil
	}
	var all []listedObject
	start := ""
	for {
		items, truncated, next, err := remote.List(ctx, prefix, start)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if !truncated || next == "" || next == start {
			break
		}
		start = next
	}
	return all, nil
}

func (e *Engine) downloadJob(ctx context.Context, task *Task, index int, remote remote, used map[string]struct{}) error {
	job := &task.Files[index]
	relKey := job.Key
	if strings.HasSuffix(task.Key, "/") && strings.HasPrefix(job.Key, task.Key) {
		relKey = strings.TrimPrefix(job.Key, task.Key)
	}
	local := job.Local
	if local == "" {
		var notes []PathNote
		local, notes = MapDownloadPath(relKey, task.DestPath, e.osName, used, e.longPaths)
		task.Report = append(task.Report, notes...)
	}
	if local == "" {
		job.Done = true
		e.saveFile(task, index)
		return nil
	}
	continuing := job.Offset > 0
	if !continuing {
		if st, err := os.Stat(local + ".dspart"); err == nil && st.Size() > 0 {
			continuing = true
			job.Offset = st.Size()
		}
	}
	if !continuing {
		if _, err := os.Stat(local); err == nil {
			choice := task.Conflict
			if choice == "" || choice == "ask" {
				task.State = "conflict"
				task.PendingKey = job.Key
				task.PendingLocal = local
				e.save(task)
				d, err := e.waitDecision(ctx, task.ID)
				if err != nil {
					return err
				}
				choice = d.choice
				if d.all {
					task.Conflict = choice
				}
				task.State = "running"
				task.PendingKey = ""
				task.PendingLocal = ""
			}
			switch choice {
			case "skip":
				task.Report = append(task.Report, PathNote{Action: "skip", From: job.Key, To: local, Reason: "exists"})
				job.Done = true
				e.saveFile(task, index)
				return nil
			case "rename":
				local = uniqueLocal(local, e.osName, used)
				task.Report = append(task.Report, PathNote{Action: "rename", From: job.Key, To: local, Reason: "exists"})
			case "replace":
			default:
				task.Report = append(task.Report, PathNote{Action: "skip", From: job.Key, Reason: "exists"})
				job.Done = true
				e.saveFile(task, index)
				return nil
			}
		}
	}
	job.Local = local
	used[foldName(local, e.osName)] = struct{}{}
	restarts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := e.copyObject(ctx, task, index, remote)
		if err == nil {
			e.mu.Lock()
			job.Done = true
			job.Offset = job.Size
			if job.Size > 0 && task.BytesDone < sumJobSizes(task.Files) {
				task.BytesDone = confirmedBytes(task, e.limits.PartSize)
			}
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
			return nil
		}
		if errors.Is(err, errNoRange) {
			_ = os.Remove(local + ".dspart")
			e.mu.Lock()
			job.Offset = 0
			task.ResumeMode = "restart"
			task.Message = "presign_unavailable"
			task.BytesDone = confirmedBytes(task, e.limits.PartSize)
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
			restarts++
			if restarts > 2 {
				return err
			}
			continue
		}
		if errors.Is(err, errETagRestart) {
			_ = os.Remove(local + ".dspart")
			e.mu.Lock()
			job.Offset = 0
			task.Message = "etag_mismatch"
			task.BytesDone = confirmedBytes(task, e.limits.PartSize)
			e.saveFileLocked(task, index)
			e.mu.Unlock()
			e.flushProgress()
			restarts++
			if restarts > 1 {
				return err
			}
			continue
		}
		return err
	}
}

func (e *Engine) copyObject(ctx context.Context, task *Task, index int, remote remote) error {
	job := &task.Files[index]
	tmp := job.Local + ".dspart"
	if job.Offset > 0 {
		if st, err := os.Stat(tmp); err == nil && st.Size() != job.Offset {
			_ = os.Truncate(tmp, job.Offset)
		}
	}
	body, etag, nlen, _, err := remote.Open(ctx, job.Key, job.Offset)
	if err != nil {
		return err
	}
	defer body.Close()
	if etag != "" && job.ETag != "" && etag != job.ETag {
		return errETagRestart
	}
	if job.ETag == "" && etag != "" {
		job.ETag = etag
	}
	if job.Size == 0 && job.Offset == 0 && nlen > 0 {
		job.Size = nlen
		task.Size += nlen
	}
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_RDWR
	if job.Offset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(extendPath(tmp), flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if job.Offset > 0 {
		if _, err := f.Seek(job.Offset, io.SeekStart); err != nil {
			return err
		}
	}
	buf := make([]byte, 32*1024)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				e.saveFile(task, index)
				return werr
			}
			e.mu.Lock()
			job.Offset += int64(n)
			task.Offset = job.Offset
			e.mu.Unlock()
			e.addProgress(task, index, int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			e.saveFile(task, index)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return wrapNet(rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if job.Size > 0 && job.Offset != job.Size && job.ETag == "" {
		return fmt.Errorf("size")
	}
	if job.Size > 0 && job.Offset != job.Size && etag != "" && job.ETag != "" && etag == job.ETag && job.Offset < job.Size {
		return fmt.Errorf("size")
	}
	if err := os.Rename(tmp, job.Local); err != nil {
		_ = os.Remove(job.Local)
		if err := os.Rename(tmp, job.Local); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) waitDecision(ctx context.Context, id string) (decision, error) {
	e.mu.Lock()
	live := e.live[id]
	e.mu.Unlock()
	if live == nil {
		return decision{}, fmt.Errorf("transfer is not waiting")
	}
	select {
	case d := <-live.decide:
		return d, nil
	case <-ctx.Done():
		return decision{}, ctx.Err()
	}
}

func uniqueLocal(path string, osName OSName, used map[string]struct{}) string {
	for n := 2; n < 10000; n++ {
		next := suffixed(path, n, osName)
		if _, ok := used[foldName(next, osName)]; !ok {
			return next
		}
	}
	return path + "_"
}

func (a *App) remoteFor(c creds, bucket string) (remote, error) {
	remote, err := a.openRemote(c.Profile, c.Token, c.AccessKey, c.Secret)
	if err != nil {
		return nil, err
	}
	if rest, ok := remote.(*restRemote); ok {
		rest.bucket = bucket
	}
	if s3, ok := remote.(*s3Remote); ok {
		s3.bucket = bucket
	}
	return remote, nil
}

func (a *App) openRemote(profile Profile, token, accessKey, secret string) (remote, error) {
	if profile.AuthMethod == "access_key" && secret != "" {
		client, err := clientFor(profile)
		if err != nil {
			return nil, err
		}
		region := "us-east-1"
		return &s3Remote{
			base:      client.BaseURL,
			accessKey: accessKey,
			secret:    secret,
			region:    region,
			client:    client.Stream,
			assemble:  client.Assemble,
		}, nil
	}
	client, err := clientFor(profile)
	if err != nil {
		return nil, err
	}
	client.Token = token
	rangeREST := false
	if profile.Server != nil && profile.Server.Features != nil && profile.Server.Features["objects.range"] > 0 {
		rangeREST = true
	}
	return &restRemote{client: client, rangeREST: rangeREST}, nil
}

func safeTransferMessage(err error) string {
	if err == nil {
		return ""
	}
	var tlsFail *tlsError
	if errors.As(err, &tlsFail) && tlsFail != nil {
		return tlsFail.Error()
	}
	if f, ok := err.(*apiFault); ok {
		if f.Code != "" {
			return publicError(f.Code)
		}
	}
	if errors.Is(err, errNoRange) {
		return "presign_unavailable"
	}
	if errors.Is(err, errETagRestart) {
		return "etag_mismatch"
	}
	return publicError(err.Error())
}

func (a *App) transferEnqueue(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	profile, err := a.profileFromParams(params)
	if err != nil {
		return nil, err
	}
	token, _ := params["token"].(string)
	secret, _ := params["access_secret"].(string)
	accessKey, _ := params["access_key"].(string)
	if accessKey == "" {
		accessKey = profile.Username
	}
	task, err := eng.Enqueue(enqueueReq{
		Profile:   profile,
		Token:     token,
		AccessKey: accessKey,
		Secret:    secret,
		Direction: strParam(params, "direction"),
		Bucket:    strParam(params, "bucket"),
		Key:       strParam(params, "key"),
		LocalPath: strParam(params, "local_path"),
		DestPath:  strParam(params, "dest_path"),
		Follow:    boolParam(params, "follow_symlinks"),
		Conflict:  strParam(params, "conflict"),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"transfer": task}, nil
}

func (a *App) transferResume(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	profile, err := a.profileFromParams(params)
	if err != nil {
		return nil, err
	}
	token, _ := params["token"].(string)
	secret, _ := params["access_secret"].(string)
	accessKey, _ := params["access_key"].(string)
	if accessKey == "" {
		accessKey = profile.Username
	}
	n, err := eng.Resume(profile, token, accessKey, secret)
	if err != nil {
		return nil, err
	}
	return map[string]any{"resumed": n}, nil
}

func (a *App) transferRetry(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	profile, err := a.profileFromParams(params)
	if err != nil {
		return nil, err
	}
	token, _ := params["token"].(string)
	secret, _ := params["access_secret"].(string)
	accessKey, _ := params["access_key"].(string)
	if accessKey == "" {
		accessKey = profile.Username
	}
	id := strParam(params, "transfer_id")
	if err := eng.Retry(profile, id, token, accessKey, secret); err != nil {
		return nil, err
	}
	task, err := eng.journal.Get(id)
	if err != nil {
		return nil, err
	}
	scrubTask(&task)
	task.Files = nil
	return map[string]any{"transfer": task}, nil
}

func (a *App) transferList(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	items, err := eng.List()
	if err != nil {
		return nil, err
	}
	profileID := strParam(params, "id")
	out := make([]Task, 0, len(items))
	for i := range items {
		if !taskVisible(profileID, items[i].ProfileID) {
			continue
		}
		scrubTask(&items[i])
		items[i].Files = nil
		items[i].Report = nil
		out = append(out, items[i])
	}
	return map[string]any{"transfers": out}, nil
}

// taskVisible is the transfer.list scope. A named profile sees its own rows
// and rows written before profile_id existed. Another profile's rows stay
// out. An empty id is the whole journal; the window always sends the id.
func taskVisible(requested, stored string) bool {
	if requested == "" {
		return true
	}
	return stored == "" || stored == requested
}

func (a *App) transferFiles(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	id := strParam(params, "transfer_id")
	if id == "" {
		return nil, fmt.Errorf("transfer id is required")
	}
	task, err := eng.journal.Get(id)
	if err != nil {
		return nil, fmt.Errorf("transfer was not found")
	}
	files := make([]map[string]any, 0, len(task.Files))
	for i := range task.Files {
		job := task.Files[i]
		files = append(files, map[string]any{
			"key":   job.Key,
			"local": job.Local,
			"size":  job.Size,
			"done":  job.Done,
		})
	}
	scrubTask(&task)
	return map[string]any{"detail": map[string]any{
		"id":          task.ID,
		"report":      task.Report,
		"files":       files,
		"files_done":  task.FilesDone,
		"files_total": task.FilesTotal,
	}}, nil
}

func (a *App) transferCancel(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	profile, err := a.profileFromParams(params)
	if err != nil {
		return nil, err
	}
	id := strParam(params, "transfer_id")
	if id == "" {
		return nil, fmt.Errorf("transfer id is required")
	}
	token, _ := params["token"].(string)
	secret, _ := params["access_secret"].(string)
	accessKey, _ := params["access_key"].(string)
	if accessKey == "" {
		accessKey = profile.Username
	}
	if err := eng.cancel(id, creds{Profile: profile, Token: token, AccessKey: accessKey, Secret: secret}); err != nil {
		return nil, err
	}
	task, err := eng.journal.Get(id)
	if err != nil {
		return map[string]any{"cancelled": id}, nil
	}
	return map[string]any{"transfer": task}, nil
}

func (a *App) transferResolve(params map[string]any) (any, error) {
	eng, err := a.engine()
	if err != nil {
		return nil, err
	}
	id := strParam(params, "transfer_id")
	if err := eng.Resolve(id, strParam(params, "choice"), boolParam(params, "apply_all")); err != nil {
		return nil, err
	}
	return map[string]any{"resolved": id}, nil
}
