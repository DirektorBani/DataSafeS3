package migrate

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// stateKey identifies one source object version.
type stateKey struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
}

type stateRow struct {
	stateKey
	Status string `json:"status"`
	ETag   string `json:"etag,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// State is the resumable JSONL journal in --state.
type State struct {
	path string
	mu   sync.Mutex
	done map[string]stateRow
	f    *os.File
}

func OpenState(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "objects.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	st := &State{path: path, done: map[string]stateRow{}, f: f}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row stateRow
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if row.Status == "copied" || row.Status == "skipped" {
			st.done[row.id()] = row
		}
	}
	if err := sc.Err(); err != nil {
		f.Close()
		return nil, err
	}
	return st, nil
}

func (s *State) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func (s *State) Has(bucket, key, versionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.done[(stateKey{Bucket: bucket, Key: key, VersionID: versionID}).id()]
	return ok
}

func (s *State) Mark(bucket, key, versionID, status, etag string, size int64) error {
	row := stateRow{
		stateKey: stateKey{Bucket: bucket, Key: key, VersionID: versionID},
		Status:   status,
		ETag:     etag,
		Size:     size,
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.Write(raw); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	s.done[row.id()] = row
	return nil
}

func (k stateKey) id() string {
	return k.Bucket + "\n" + k.Key + "\n" + k.VersionID
}
