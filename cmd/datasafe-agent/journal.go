package main

import (
	"encoding/json"
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
		_, err := tx.CreateBucketIfNotExists([]byte("tasks"))
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
	task.UpdatedUnix = time.Now().Unix()
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return j.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tasks")).Put([]byte(task.ID), raw)
	})
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
	return task, err
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
