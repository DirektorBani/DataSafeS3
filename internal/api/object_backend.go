package api

import (
	"path/filepath"

	"github.com/DirektorBani/datasafe/internal/storage"
	"github.com/DirektorBani/datasafe/internal/storage/erasure"
)

func openObjectBackend(cfg storage.ObjectBackendConfig) (storage.ObjectBackend, error) {
	objectDir := filepath.Join(cfg.DataDir, "objects")
	switch cfg.Backend {
	case "erasure":
		return erasure.OpenBackend(erasure.Config{
			Paths:     cfg.Paths,
			Layout:    cfg.Layout,
			ReadOnly:  cfg.ReadOnly,
			Staging:   objectDir,
			HealEvery: cfg.HealEvery,
		})
	default:
		if cfg.ReadOnly {
			return storage.OpenFSBackend(objectDir)
		}
		return storage.NewFSBackend(objectDir)
	}
}
