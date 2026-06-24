package syncapp

import (
	"encoding/json"
	"time"
)

// SyncProgress is emitted during RunOnce when OnProgress is set.
type SyncProgress struct {
	Phase    string `json:"phase"` // push, pull, delete
	File     string `json:"file,omitempty"`
	Action   string `json:"action,omitempty"` // upload, download, skip, delete, conflict
	Uploaded int    `json:"uploaded"`
	Downloaded int  `json:"downloaded"`
	Skipped  int    `json:"skipped"`
}

// SyncResult is the outcome of one sync pass.
type SyncResult struct {
	Uploaded      int        `json:"uploaded"`
	Downloaded    int        `json:"downloaded"`
	Skipped       int        `json:"skipped"`
	DeletedLocal  int        `json:"deleted_local"`
	DeletedRemote int        `json:"deleted_remote"`
	Conflicts     []Conflict `json:"conflicts,omitempty"`
	Errors        []string   `json:"errors,omitempty"`
}

func (r SyncResult) JSON() ([]byte, error) {
	return json.Marshal(r)
}

// ProfileSnapshot for status --json.
type ProfileSnapshot struct {
	ProfileName string `json:"profile"`
	Profile     Profile `json:"profile_detail"`
	Tracked     int    `json:"tracked_files"`
	LoggedIn    bool   `json:"logged_in"`
}

func ProfileStatus(profileName string) (ProfileSnapshot, error) {
	st, err := LoadState(profileName)
	if err != nil {
		return ProfileSnapshot{}, err
	}
	return ProfileSnapshot{
		ProfileName: profileName,
		Profile:     st.Profile,
		Tracked:     len(st.Files),
		LoggedIn:    st.Profile.Token != "",
	}, nil
}

// ProgressEvent for JSON-lines streaming.
type ProgressEvent struct {
	Type      string        `json:"type"` // progress, done, error
	Progress  *SyncProgress `json:"progress,omitempty"`
	Result    *SyncResult   `json:"result,omitempty"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

func NewProgressEvent(p SyncProgress) ProgressEvent {
	return ProgressEvent{Type: "progress", Progress: &p, Timestamp: time.Now().UTC()}
}

func NewDoneEvent(r SyncResult) ProgressEvent {
	return ProgressEvent{Type: "done", Result: &r, Timestamp: time.Now().UTC()}
}

func NewErrorEvent(msg string) ProgressEvent {
	return ProgressEvent{Type: "error", Error: msg, Timestamp: time.Now().UTC()}
}
