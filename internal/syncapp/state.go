package syncapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type FileState struct {
	ETag         string    `json:"etag,omitempty"`
	Size         int64     `json:"size"`
	Modified     time.Time `json:"modified"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

type Profile struct {
	ServerURL string `json:"server_url"`
	Username  string `json:"username"`
	Token     string `json:"token"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Folder    string `json:"folder"`
}

type State struct {
	Profile Profile              `json:"profile"`
	Files   map[string]FileState `json:"files"`
}

func statePath(profileName string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "datasafe", "sync-"+profileName+".json"), nil
}

func LoadState(profileName string) (*State, error) {
	p, err := statePath(profileName)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Files: map[string]FileState{}}, nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	if st.Files == nil {
		st.Files = map[string]FileState{}
	}
	return &st, nil
}

func (s *State) Save(profileName string) error {
	p, err := statePath(profileName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}
