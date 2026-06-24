package syncapp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	ProfileName    string
	Client         *Client
	Folder         string
	Bucket         string
	Prefix         string
	Pull           bool
	Push           bool
	DeleteSync     bool
	ConflictPolicy ConflictPolicy
	OnProgress     func(SyncProgress)
}

func RunOnce(opts Options) (SyncResult, error) {
	if opts.Client == nil {
		return SyncResult{}, fmt.Errorf("client required")
	}
	if opts.Folder == "" || opts.Bucket == "" {
		return SyncResult{}, fmt.Errorf("folder and bucket required")
	}
	if opts.ConflictPolicy == "" {
		opts.ConflictPolicy = ConflictLastWriteWins
	}
	prefix := NormalizePrefix(opts.Prefix)
	st, err := LoadState(opts.ProfileName)
	if err != nil {
		return SyncResult{}, err
	}
	st.Profile = Profile{
		ServerURL: opts.Client.BaseURL,
		Username:  st.Profile.Username,
		Token:     opts.Client.Token,
		Bucket:    opts.Bucket,
		Prefix:    prefix,
		Folder:    opts.Folder,
	}

	var res SyncResult
	emit := func(p SyncProgress) {
		p.Uploaded = res.Uploaded
		p.Downloaded = res.Downloaded
		p.Skipped = res.Skipped
		if opts.OnProgress != nil {
			opts.OnProgress(p)
		}
	}

	remote, err := opts.Client.ListAllObjects(opts.Bucket, prefix)
	if err != nil {
		return res, err
	}
	remoteByRel := map[string]ObjectItem{}
	for _, o := range remote {
		rel, ok := relativeKey(o.Key, prefix)
		if !ok || rel == "" {
			continue
		}
		remoteByRel[rel] = o
	}

	localFiles := map[string]os.FileInfo{}
	if opts.Push || opts.DeleteSync {
		_ = filepath.WalkDir(opts.Folder, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			rel, err := filepath.Rel(opts.Folder, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, conflictsDirName+"/") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			localFiles[rel] = info
			return nil
		})
	}

	if opts.Push {
		for rel, info := range localFiles {
			p := filepath.Join(opts.Folder, filepath.FromSlash(rel))
			localHash, err := fileHash(p)
			if err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			prev := st.Files[rel]
			remoteKey := prefix + rel
			remoteObj, hasRemote := remoteByRel[rel]
			localMod := info.ModTime().UTC()

			if hasRemote {
				remoteETag := strings.Trim(remoteObj.ETag, "\"")
				if localHash == remoteETag {
					res.Skipped++
					emit(SyncProgress{Phase: "push", File: rel, Action: "skip"})
					continue
				}
				if isConflict(localHash, localMod, remoteObj, prev) {
					policy := pickWinner(opts.ConflictPolicy, localMod, remoteObj.LastModified.UTC())
					switch policy {
					case ConflictRemoteWins:
						res.Skipped++
						emit(SyncProgress{Phase: "push", File: rel, Action: "conflict_remote_wins"})
						continue
					case ConflictKeepBoth:
						data, err := opts.Client.DownloadObject(opts.Bucket, remoteKey)
						if err != nil {
							res.Errors = append(res.Errors, rel+": "+err.Error())
							continue
						}
						backup, err := saveRemoteConflictCopy(opts.Folder, rel, remoteObj.LastModified, data)
						if err != nil {
							res.Errors = append(res.Errors, rel+": "+err.Error())
							continue
						}
						res.Conflicts = append(res.Conflicts, Conflict{
							RelativePath: rel,
							LocalPath:    p,
							RemoteKey:    remoteKey,
							LocalModTime: localMod,
							RemoteMod:    remoteObj.LastModified.UTC(),
							Policy:       string(ConflictKeepBoth),
							BackupPath:   backup,
						})
						emit(SyncProgress{Phase: "push", File: rel, Action: "conflict"})
					default:
						// local_wins or last_write_wins -> local wins path falls through to upload
					}
				} else if hasRemote && localMod.Before(remoteObj.LastModified.UTC()) && prev.ETag == strings.Trim(remoteObj.ETag, "\"") {
					res.Skipped++
					emit(SyncProgress{Phase: "push", File: rel, Action: "skip"})
					continue
				}
			}

			data, err := os.ReadFile(p)
			if err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			if err := opts.Client.UploadObject(opts.Bucket, remoteKey, "", data); err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			st.Files[rel] = FileState{
				ETag:         localHash,
				Size:         info.Size(),
				Modified:     localMod,
				LastSyncedAt: time.Now().UTC(),
			}
			res.Uploaded++
			emit(SyncProgress{Phase: "push", File: rel, Action: "upload"})
		}
	}

	if opts.Pull {
		for rel, obj := range remoteByRel {
			localPath := filepath.Join(opts.Folder, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			info, statErr := os.Stat(localPath)
			prev := st.Files[rel]
			remoteETag := strings.Trim(obj.ETag, "\"")

			if statErr == nil && !info.IsDir() {
				localHash, _ := fileHash(localPath)
				localMod := info.ModTime().UTC()
				if localHash == remoteETag {
					res.Skipped++
					emit(SyncProgress{Phase: "pull", File: rel, Action: "skip"})
					continue
				}
				if isConflict(localHash, localMod, obj, prev) {
					policy := pickWinner(opts.ConflictPolicy, localMod, obj.LastModified.UTC())
					switch policy {
					case ConflictLocalWins:
						res.Skipped++
						emit(SyncProgress{Phase: "pull", File: rel, Action: "conflict_local_wins"})
						continue
					case ConflictKeepBoth:
						data, err := opts.Client.DownloadObject(opts.Bucket, prefix+rel)
						if err != nil {
							res.Errors = append(res.Errors, rel+": "+err.Error())
							continue
						}
						backup, err := saveRemoteConflictCopy(opts.Folder, rel, obj.LastModified, data)
						if err != nil {
							res.Errors = append(res.Errors, rel+": "+err.Error())
							continue
						}
						res.Conflicts = append(res.Conflicts, Conflict{
							RelativePath: rel,
							LocalPath:    localPath,
							RemoteKey:    prefix + rel,
							LocalModTime: localMod,
							RemoteMod:    obj.LastModified.UTC(),
							Policy:       string(ConflictKeepBoth),
							BackupPath:   backup,
						})
						emit(SyncProgress{Phase: "pull", File: rel, Action: "conflict"})
						continue
					default:
						// remote wins -> download below
					}
				} else if localMod.After(obj.LastModified.UTC()) && prev.ETag != "" {
					res.Skipped++
					emit(SyncProgress{Phase: "pull", File: rel, Action: "skip"})
					continue
				}
			}

			data, err := opts.Client.DownloadObject(opts.Bucket, prefix+rel)
			if err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			if err := os.WriteFile(localPath, data, 0o644); err != nil {
				res.Errors = append(res.Errors, rel+": "+err.Error())
				continue
			}
			_ = os.Chtimes(localPath, obj.LastModified, obj.LastModified)
			hash := sha256.Sum256(data)
			st.Files[rel] = FileState{
				ETag:         hex.EncodeToString(hash[:]),
				Size:         int64(len(data)),
				Modified:     obj.LastModified.UTC(),
				LastSyncedAt: time.Now().UTC(),
			}
			res.Downloaded++
			emit(SyncProgress{Phase: "pull", File: rel, Action: "download"})
		}
	}

	if opts.DeleteSync {
		for rel := range st.Files {
			_, localOK := localFiles[rel]
			_, remoteOK := remoteByRel[rel]
			localPath := filepath.Join(opts.Folder, filepath.FromSlash(rel))
			remoteKey := prefix + rel

			if opts.Push && !localOK && remoteOK {
				if err := opts.Client.DeleteObject(opts.Bucket, remoteKey); err != nil {
					res.Errors = append(res.Errors, rel+": delete remote: "+err.Error())
					continue
				}
				delete(st.Files, rel)
				res.DeletedRemote++
				emit(SyncProgress{Phase: "delete", File: rel, Action: "delete_remote"})
			}
			if opts.Pull && localOK && !remoteOK {
				if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
					res.Errors = append(res.Errors, rel+": delete local: "+err.Error())
					continue
				}
				delete(st.Files, rel)
				res.DeletedLocal++
				emit(SyncProgress{Phase: "delete", File: rel, Action: "delete_local"})
			}
			if !localOK && !remoteOK {
				delete(st.Files, rel)
			}
		}
	}

	if err := st.Save(opts.ProfileName); err != nil {
		return res, err
	}
	return res, nil
}

func NormalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func relativeKey(key, prefix string) (string, bool) {
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		return "", false
	}
	return strings.TrimPrefix(key, prefix), true
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// RunOnceJSON runs sync and prints JSON-lines progress to w.
func RunOnceJSON(opts Options, w interface{ Write([]byte) (int, error) }) (SyncResult, error) {
	opts.OnProgress = func(p SyncProgress) {
		ev := NewProgressEvent(p)
		raw, _ := json.Marshal(ev)
		_, _ = w.Write(append(raw, '\n'))
	}
	res, err := RunOnce(opts)
	if err != nil {
		ev := NewErrorEvent(err.Error())
		raw, _ := json.Marshal(ev)
		_, _ = w.Write(append(raw, '\n'))
		return res, err
	}
	done := NewDoneEvent(res)
	raw, _ := json.Marshal(done)
	_, _ = w.Write(append(raw, '\n'))
	return res, nil
}
