package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

type Journal struct {
	db *bolt.DB
}

func openJournal(path string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
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
	j := &Journal{db: db}
	if err := j.interruptActive(); err != nil {
		_ = db.Close()
		return nil, err
	}
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

// SaveMeta writes the task row without its file list. Directory progress
// must not rewrite every file on each update.
func (j *Journal) SaveMeta(task Task) error {
	task.Files = nil
	task.UpdatedUnix = time.Now().Unix()
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return j.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tasks")).Put([]byte(task.ID), raw)
	})
}

func fileStorageKey(taskID string, index int) []byte {
	return []byte(fmt.Sprintf("%s\x00%08d", taskID, index))
}

// PutFiles replaces the per-file rows for one task.
func (j *Journal) PutFiles(taskID string, files []fileJob) error {
	prefix := []byte(taskID + "\x00")
	return j.db.Update(func(tx *bolt.Tx) error {
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
}

// SaveFileProgress writes one file row and the task summary in one commit.
func (j *Journal) SaveFileProgress(task Task, index int) error {
	if index < 0 || index >= len(task.Files) {
		return j.SaveMeta(task)
	}
	job, err := json.Marshal(task.Files[index])
	if err != nil {
		return err
	}
	task.Files = nil
	task.UpdatedUnix = time.Now().Unix()
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	key := fileStorageKey(task.ID, index)
	return j.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket([]byte("files")).Put(key, job); err != nil {
			return err
		}
		return tx.Bucket([]byte("tasks")).Put([]byte(task.ID), raw)
	})
}

func (j *Journal) LoadFiles(taskID string) ([]fileJob, error) {
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

func (j *Journal) Get(id string) (Task, error) {
	var task Task
	err := j.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("tasks")).Get([]byte(id))
		if raw == nil {
			return os.ErrNotExist
		}
		return json.Unmarshal(raw, &task)
	})
	if err != nil {
		return Task{}, err
	}
	rows, err := j.LoadFiles(id)
	if err != nil {
		return Task{}, err
	}
	if len(rows) > 0 {
		task.Files = rows
	}
	return task, nil
}

func (j *Journal) List() ([]Task, error) {
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
	return out, err
}
