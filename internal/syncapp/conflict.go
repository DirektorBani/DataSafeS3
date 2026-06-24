package syncapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConflictPolicy controls behaviour when local and remote both changed since last sync.
type ConflictPolicy string

const (
	ConflictLastWriteWins ConflictPolicy = "last_write_wins"
	ConflictLocalWins     ConflictPolicy = "local_wins"
	ConflictRemoteWins    ConflictPolicy = "remote_wins"
	ConflictKeepBoth      ConflictPolicy = "keep_both"
)

func ParseConflictPolicy(s string) ConflictPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "local_wins", "local":
		return ConflictLocalWins
	case "remote_wins", "remote":
		return ConflictRemoteWins
	case "keep_both", "both":
		return ConflictKeepBoth
	default:
		return ConflictLastWriteWins
	}
}

// Conflict records a file where both sides diverged.
type Conflict struct {
	RelativePath string    `json:"relative_path"`
	LocalPath    string    `json:"local_path,omitempty"`
	RemoteKey    string    `json:"remote_key"`
	LocalModTime time.Time `json:"local_mod_time,omitempty"`
	RemoteMod    time.Time `json:"remote_mod_time,omitempty"`
	Policy       string    `json:"policy,omitempty"`
	BackupPath   string    `json:"backup_path,omitempty"`
	ResolvedAt   time.Time `json:"resolved_at,omitempty"`
}

const conflictsDirName = ".datasafe-conflicts"

func conflictsDir(folder string) string {
	return filepath.Join(folder, conflictsDirName)
}

func conflictBackupPathFlat(folder, rel string, remoteMod time.Time) string {
	safe := strings.ReplaceAll(rel, "/", "__")
	ext := filepath.Ext(safe)
	stem := strings.TrimSuffix(safe, ext)
	tag := remoteMod.UTC().Format("2006-01-02-150405")
	name := fmt.Sprintf("%s (conflict %s)%s", stem, tag, ext)
	return filepath.Join(conflictsDir(folder), name)
}

// saveRemoteConflictCopy writes remote bytes into the conflicts backup folder.
func saveRemoteConflictCopy(folder, rel string, remoteMod time.Time, data []byte) (string, error) {
	dest := conflictBackupPathFlat(folder, rel, remoteMod)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// ListConflicts returns unresolved conflict backups under the local sync folder.
func ListConflicts(folder string) ([]string, error) {
	dir := conflictsDir(folder)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ResolveConflict removes a named backup from the conflicts folder.
func ResolveConflict(folder, name string) error {
	p := filepath.Join(conflictsDir(folder), filepath.Base(name))
	if err := os.Remove(p); err != nil {
		return err
	}
	return nil
}

func isConflict(localHash string, localMod time.Time, remote ObjectItem, prev FileState) bool {
	remoteETag := strings.Trim(remote.ETag, "\"")
	if localHash == remoteETag {
		return false
	}
	if prev.ETag == "" && prev.LastSyncedAt.IsZero() {
		return false
	}
	localChanged := prev.ETag != localHash || localMod.After(prev.LastSyncedAt)
	remoteChanged := prev.ETag != remoteETag || remote.LastModified.UTC().After(prev.LastSyncedAt)
	return localChanged && remoteChanged
}

func pickWinner(policy ConflictPolicy, localMod, remoteMod time.Time) ConflictPolicy {
	if policy != ConflictLastWriteWins {
		return policy
	}
	if localMod.UTC().After(remoteMod.UTC()) {
		return ConflictLocalWins
	}
	return ConflictRemoteWins
}
