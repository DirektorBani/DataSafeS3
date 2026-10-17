package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// active states that belong to a running process. After the agent starts
// again they become interrupted: continuing them is E06, and the rows stay
// so that step can read the confirmed parts.
var activeStates = map[string]bool{
	"queued":          true,
	"running":         true,
	"waiting_network": true,
	"conflict":        true,
}

type partRec struct {
	Number int    `json:"number"`
	ETag   string `json:"etag"`
}

type fileJob struct {
	Local     string    `json:"local"`
	Key       string    `json:"key"`
	Size      int64     `json:"size"`
	MtimeUnix int64     `json:"mtime_unix"`
	UploadID  string    `json:"upload_id,omitempty"`
	Parts     []partRec `json:"parts,omitempty"`
	Done      bool      `json:"done,omitempty"`
	ETag      string    `json:"etag,omitempty"`
	Offset    int64     `json:"offset,omitempty"`
}

// Task is one transfer row. It has no token, password, or access-key secret.
type Task struct {
	ID             string     `json:"id"`
	ProfileID      string     `json:"profile_id"`
	Direction      string     `json:"direction"`
	State          string     `json:"state"`
	LocalPath      string     `json:"local_path"`
	DestPath       string     `json:"dest_path,omitempty"`
	Bucket         string     `json:"bucket"`
	Key            string     `json:"key"`
	Size           int64      `json:"size"`
	MtimeUnix      int64      `json:"mtime_unix"`
	UploadID       string     `json:"upload_id,omitempty"`
	Parts          []partRec  `json:"parts,omitempty"`
	Offset         int64      `json:"offset,omitempty"`
	ETag           string     `json:"etag,omitempty"`
	Message        string     `json:"message,omitempty"`
	Report         []PathNote `json:"report,omitempty"`
	Files          []fileJob  `json:"files,omitempty"`
	FilesDone      int        `json:"files_done,omitempty"`
	FilesTotal     int        `json:"files_total,omitempty"`
	FollowSymlinks bool       `json:"follow_symlinks,omitempty"`
	Conflict       string     `json:"conflict,omitempty"`
	ResumeMode     string     `json:"resume_mode,omitempty"`
	BytesDone      int64      `json:"bytes_done,omitempty"`
	PendingKey     string     `json:"pending_key,omitempty"`
	PendingLocal   string     `json:"pending_local,omitempty"`
	CreatedUnix    int64      `json:"created_unix"`
	UpdatedUnix    int64      `json:"updated_unix"`
}

const (
	// journalBatchFiles is how many per-file updates share one fsync.
	// One bolt row per file stays; only the sync is batched.
	journalBatchFiles = 32
	journalFlushEvery = 200 * time.Millisecond
)

type queuedTask struct {
	meta    Task
	hasMeta bool
	files   map[int]fileJob
}

type Journal struct {
	db         *bolt.DB
	mu         sync.Mutex
	writeMu    sync.Mutex
	batch      map[string]*queuedTask
	inflight   map[string]*queuedTask
	queued     int
	lastFlush  time.Time
	flushEvery time.Duration
	batchSize  int
	syncs      int
}

func openJournal(path string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	// File-row updates are committed in batches. NoSync keeps each commit
	// in memory; Flush calls Sync once per batch instead of once per file.
	db.NoSync = true
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("tasks")); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte("files"))
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	j := &Journal{
		db:         db,
		batch:      map[string]*queuedTask{},
		inflight:   map[string]*queuedTask{},
		lastFlush:  time.Now(),
		flushEvery: journalFlushEvery,
		batchSize:  journalBatchFiles,
	}
	if err := j.interruptActive(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := j.db.Sync(); err != nil {
		_ = db.Close()
		return nil, err
	}
	j.syncs = 1
	return j, nil
}

func (j *Journal) interruptActive() error {
	now := time.Now().Unix()
	return j.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tasks"))
		return b.ForEach(func(k, v []byte) error {
			var task Task
			if err := json.Unmarshal(v, &task); err != nil {
				return nil
			}
			if !activeStates[task.State] {
				return nil
			}
			task.State = "interrupted"
			task.Message = "resume_after_restart"
			task.UpdatedUnix = now
			raw, err := json.Marshal(task)
			if err != nil {
				return err
			}
			return b.Put(k, raw)
		})
	})
}

func (j *Journal) Save(task Task) error {
	files := task.Files
	if err := j.SaveMeta(task); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return j.PutFiles(task.ID, files)
}

// SaveMeta writes the task row without its file list and syncs the pending
// file rows. State changes go through here so a cancel or a finished task
// is durable. Directory progress must not rewrite every file on each update.
func (j *Journal) SaveMeta(task Task) error {
	task.Files = nil
	task.UpdatedUnix = time.Now().Unix()
	j.mu.Lock()
	q := j.ensure(task.ID)
	q.meta = task
	q.hasMeta = true
	j.mu.Unlock()
	return j.commit(true)
}

// queueFile remembers one file row and the task summary. The bolt write
// waits for the batch. Get and List overlay this map, so progress stays
// visible before the sync.
func (j *Journal) queueFile(task Task, index int) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	q := j.ensure(task.ID)
	if index >= 0 && index < len(task.Files) {
		job := task.Files[index]
		job.Parts = append([]partRec(nil), job.Parts...)
		if q.files == nil {
			q.files = map[int]fileJob{}
		}
		q.files[index] = job
		j.queued++
	}
	task.Files = nil
	task.UpdatedUnix = time.Now().Unix()
	q.meta = task
	q.hasMeta = true
	return nil
}

func (j *Journal) ensure(id string) *queuedTask {
	q := j.batch[id]
	if q == nil {
		q = &queuedTask{files: map[int]fileJob{}}
		j.batch[id] = q
	}
	if q.files == nil {
		q.files = map[int]fileJob{}
	}
	return q
}

// Due reports that a file batch should be synced. The caller must not hold
// the transfer lock: the sync itself runs in Flush.
func (j *Journal) Due() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.dueLocked()
}

// Flush writes queued file rows and task summaries in one transaction, then
// syncs once. The disk sync does not hold the memory lock, so transfers can
// keep queueing the next files.
func (j *Journal) Flush() error {
	return j.commit(false)
}

func (j *Journal) commit(force bool) error {
	j.writeMu.Lock()
	defer j.writeMu.Unlock()
	for {
		j.mu.Lock()
		if len(j.batch) == 0 {
			j.mu.Unlock()
			return nil
		}
		if !force && !j.dueLocked() {
			j.mu.Unlock()
			return nil
		}
		batch := j.batch
		j.inflight = batch
		j.batch = map[string]*queuedTask{}
		j.queued = 0
		j.mu.Unlock()
		err := j.writeBatch(batch)
		j.mu.Lock()
		if err != nil {
			j.restore(batch)
			j.inflight = map[string]*queuedTask{}
			j.mu.Unlock()
			return err
		}
		j.inflight = map[string]*queuedTask{}
		j.mu.Unlock()
		if err := j.db.Sync(); err != nil {
			return err
		}
		j.mu.Lock()
		j.syncs++
		j.lastFlush = time.Now()
		pending := force && len(j.batch) > 0
		j.mu.Unlock()
		if !pending {
			return nil
		}
	}
}

func (j *Journal) dueLocked() bool {
	if j.queued == 0 {
		return false
	}
	size := j.batchSize
	if size < 1 {
		size = journalBatchFiles
	}
	if j.queued >= size {
		return true
	}
	every := j.flushEvery
	if every <= 0 {
		every = journalFlushEvery
	}
	return time.Since(j.lastFlush) >= every
}

func (j *Journal) restore(batch map[string]*queuedTask) {
	for id, q := range batch {
		cur := j.ensure(id)
		if q.hasMeta && !cur.hasMeta {
			cur.meta = q.meta
			cur.hasMeta = true
		}
		for index, job := range q.files {
			if _, ok := cur.files[index]; ok {
				continue
			}
			cur.files[index] = job
			j.queued++
		}
	}
}

func (j *Journal) writeBatch(batch map[string]*queuedTask) error {
	return j.db.Update(func(tx *bolt.Tx) error {
		tasks := tx.Bucket([]byte("tasks"))
		files := tx.Bucket([]byte("files"))
		for id, q := range batch {
			if q.hasMeta {
				raw, err := json.Marshal(q.meta)
				if err != nil {
					return err
				}
				if err := tasks.Put([]byte(id), raw); err != nil {
					return err
				}
			}
			for index, job := range q.files {
				raw, err := json.Marshal(job)
				if err != nil {
					return err
				}
				if err := files.Put(fileStorageKey(id, index), raw); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Syncs is how many times the journal has fsynced since it was opened.
func (j *Journal) Syncs() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.syncs
}

func fileStorageKey(taskID string, index int) []byte {
	return []byte(fmt.Sprintf("%s\x00%08d", taskID, index))
}

// PutFiles replaces the per-file rows for one task and syncs once.
func (j *Journal) PutFiles(taskID string, files []fileJob) error {
	prefix := []byte(taskID + "\x00")
	j.writeMu.Lock()
	defer j.writeMu.Unlock()
	j.mu.Lock()
	if q := j.batch[taskID]; q != nil {
		j.queued -= len(q.files)
		if j.queued < 0 {
			j.queued = 0
		}
		q.files = map[int]fileJob{}
	}
	if q := j.inflight[taskID]; q != nil {
		q.files = map[int]fileJob{}
	}
	j.mu.Unlock()
	err := j.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("files"))
		c := b.Cursor()
		var stale [][]byte
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			stale = append(stale, append([]byte(nil), k...))
		}
		for _, k := range stale {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		for i := range files {
			raw, err := json.Marshal(files[i])
			if err != nil {
				return err
			}
			if err := b.Put(fileStorageKey(taskID, i), raw); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := j.db.Sync(); err != nil {
		return err
	}
	j.mu.Lock()
	j.syncs++
	j.lastFlush = time.Now()
	j.mu.Unlock()
	return nil
}

func (j *Journal) LoadFiles(taskID string) ([]fileJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rows, err := j.loadFiles(taskID)
	if err != nil {
		return nil, err
	}
	task := Task{ID: taskID, Files: rows}
	j.overlay(&task)
	return task.Files, nil
}

func (j *Journal) loadFiles(taskID string) ([]fileJob, error) {
	prefix := []byte(taskID + "\x00")
	var out []fileJob
	err := j.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("files"))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var job fileJob
			if err := json.Unmarshal(v, &job); err != nil {
				return err
			}
			out = append(out, job)
		}
		return nil
	})
	return out, err
}

func (j *Journal) readTask(id string) (Task, error) {
	var task Task
	err := j.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("tasks")).Get([]byte(id))
		if raw == nil {
			return os.ErrNotExist
		}
		return json.Unmarshal(raw, &task)
	})
	return task, err
}

func (j *Journal) Get(id string) (Task, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	task, err := j.readTask(id)
	if err != nil {
		return Task{}, err
	}
	rows, err := j.loadFiles(id)
	if err != nil {
		return Task{}, err
	}
	if len(rows) > 0 {
		task.Files = rows
	}
	j.overlay(&task)
	return task, nil
}

func (j *Journal) overlay(task *Task) {
	j.applyQueued(task, j.inflight[task.ID])
	j.applyQueued(task, j.batch[task.ID])
}

func (j *Journal) applyQueued(task *Task, q *queuedTask) {
	if q == nil {
		return
	}
	if q.hasMeta {
		files := task.Files
		*task = q.meta
		task.Files = files
	}
	if len(q.files) == 0 {
		return
	}
	max := len(task.Files) - 1
	for index := range q.files {
		if index > max {
			max = index
		}
	}
	if max >= len(task.Files) {
		grown := make([]fileJob, max+1)
		copy(grown, task.Files)
		task.Files = grown
	}
	for index, job := range q.files {
		task.Files[index] = job
	}
}

func (j *Journal) List() ([]Task, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Task
	err := j.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tasks")).ForEach(func(_, v []byte) error {
			var task Task
			if err := json.Unmarshal(v, &task); err != nil {
				return nil
			}
			out = append(out, task)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		if q := j.inflight[out[i].ID]; q != nil && q.hasMeta {
			out[i] = q.meta
			out[i].Files = nil
		}
		if q := j.batch[out[i].ID]; q != nil && q.hasMeta {
			out[i] = q.meta
			out[i].Files = nil
		}
	}
	return out, nil
}
