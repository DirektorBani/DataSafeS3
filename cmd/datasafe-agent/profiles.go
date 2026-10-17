package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Profile is the non-secret record stored in profiles.json.
type Profile struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	ServerURL     string      `json:"server_url"`
	AuthMethod    string      `json:"auth_method"`
	Username      string      `json:"username,omitempty"`
	TLS           ProfileTLS  `json:"tls,omitempty"`
	DefaultBucket string      `json:"default_bucket,omitempty"`
	Server        *ServerCard `json:"server,omitempty"`
	Session       string      `json:"session,omitempty"`
	ExpiresAtUnix int64       `json:"expires_at_unix,omitempty"`
}

type ProfileTLS struct {
	CAFile      string `json:"ca_file,omitempty"`
	InsecureDev bool   `json:"insecure_dev,omitempty"`
}

type profileFile struct {
	Profiles []Profile `json:"profiles"`
}

func (a *App) loadProfiles() (profileFile, error) {
	path := filepath.Join(a.dataDir, "profiles.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return profileFile{}, nil
	}
	if err != nil {
		return profileFile{}, err
	}
	var file profileFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return profileFile{}, err
	}
	return file, nil
}

func (a *App) saveProfiles(file profileFile) error {
	if err := os.MkdirAll(a.dataDir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(a.dataDir, "profiles.json"), raw, 0o600)
}

func (a *App) putProfile(in Profile) (Profile, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.ServerURL = strings.TrimSpace(in.ServerURL)
	in.AuthMethod = strings.TrimSpace(in.AuthMethod)
	in.Username = strings.TrimSpace(in.Username)
	if in.Name == "" || in.ServerURL == "" {
		return Profile{}, fmt.Errorf("name and server_url are required")
	}
	switch in.AuthMethod {
	case "password", "oidc", "access_key", "api_token":
	default:
		return Profile{}, fmt.Errorf("auth_method is not supported")
	}
	if _, err := newClientConfig(in.ServerURL, in.TLS.InsecureDev, in.TLS.CAFile); err != nil {
		return Profile{}, err
	}
	file, err := a.loadProfiles()
	if err != nil {
		return Profile{}, err
	}
	if in.ID == "" {
		in.ID, err = newID()
		if err != nil {
			return Profile{}, err
		}
		in.Session = "signed_out"
		file.Profiles = append(file.Profiles, in)
	} else {
		found := false
		for i := range file.Profiles {
			if file.Profiles[i].ID == in.ID {
				prev := file.Profiles[i]
				in.Server = prev.Server
				in.Session = prev.Session
				in.ExpiresAtUnix = prev.ExpiresAtUnix
				file.Profiles[i] = in
				found = true
				break
			}
		}
		if !found {
			return Profile{}, fmt.Errorf("profile not found")
		}
	}
	if err := a.saveProfiles(file); err != nil {
		return Profile{}, err
	}
	return in, nil
}

func (a *App) listProfiles() ([]Profile, error) {
	file, err := a.loadProfiles()
	if err != nil {
		return nil, err
	}
	if file.Profiles == nil {
		return []Profile{}, nil
	}
	return file.Profiles, nil
}

func (a *App) profileByID(id string) (Profile, error) {
	file, err := a.loadProfiles()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range file.Profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("profile not found")
}

func (a *App) updateProfile(profile Profile) error {
	file, err := a.loadProfiles()
	if err != nil {
		return err
	}
	for i := range file.Profiles {
		if file.Profiles[i].ID == profile.ID {
			file.Profiles[i] = profile
			return a.saveProfiles(file)
		}
	}
	return fmt.Errorf("profile not found")
}

func (a *App) deleteProfile(id string) error {
	file, err := a.loadProfiles()
	if err != nil {
		return err
	}
	next := file.Profiles[:0]
	found := false
	for _, profile := range file.Profiles {
		if profile.ID == id {
			found = true
			continue
		}
		next = append(next, profile)
	}
	if !found {
		return fmt.Errorf("profile not found")
	}
	file.Profiles = next
	if err := a.saveProfiles(file); err != nil {
		return err
	}
	_ = os.Remove(a.cachePath(id))
	return nil
}

func (a *App) cachePath(id string) string {
	return filepath.Join(a.dataDir, "cache", id+".json")
}

func (a *App) writeCache(id string, card ServerCard) error {
	dir := filepath.Join(a.dataDir, "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(map[string]any{
		"server": card,
		"lists":  []any{},
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.cachePath(id), append(raw, '\n'), 0o600)
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
