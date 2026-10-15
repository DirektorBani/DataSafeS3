package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

// SetVaultKMS installs a Transit provider. Tests use this with a fake Vault.
func (s *Server) SetVaultKMS(src kms.DEKSource) {
	s.svc.SetVaultKMS(src)
}

// refreshVaultFromMeta builds the Transit client only when provider and address are set.
// An idle node does not start a goroutine.
func (s *Server) refreshVaultFromMeta() {
	env, locks := vaulttransit.LoadEnv()
	var stored metadata.VaultKMSConfig
	if cfg, err := s.meta.GetSystemConfig(); err == nil {
		stored = cfg.KMS
	}
	s.applyVaultConfig(vaulttransit.Merge(env, locks, stored))
}

func (s *Server) applyVaultConfig(cfg vaulttransit.Config) {
	s.vaultMu.Lock()
	defer s.vaultMu.Unlock()
	if !cfg.Enabled() {
		if s.svc.VaultKMS() != nil {
			s.svc.SetVaultKMS(nil)
			s.vaultFP = ""
		}
		return
	}
	fp := cfg.Fingerprint()
	if fp == s.vaultFP && s.svc.VaultKMS() != nil {
		return
	}
	p, err := vaulttransit.Open(cfg)
	if err != nil || p == nil {
		slog.Error("kms", "err", "vault transit client was not started")
		s.svc.SetVaultKMS(nil)
		s.vaultFP = ""
		return
	}
	s.svc.SetVaultKMS(p)
	s.vaultFP = fp
	s.vaultOnce.Do(func() {
		stop := make(chan struct{})
		s.vaultStop = stop
		go s.vaultLoop(stop)
	})
}

func (s *Server) stopVaultLoop() {
	s.vaultStopOnce.Do(func() {
		s.vaultMu.Lock()
		ch := s.vaultStop
		s.vaultMu.Unlock()
		if ch != nil {
			close(ch)
		}
	})
}

// WatchVaultConfig reloads the shared Vault config. Followers use this so a
// save on another node is visible in under two seconds.
func (s *Server) WatchVaultConfig(ctx context.Context, interval time.Duration) {
	if interval <= 0 || interval > 2*time.Second {
		interval = 2 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.withBackgroundDB(func() { s.refreshVaultFromMeta() })
		}
	}
}

func (s *Server) watchVaultConfig(ctx context.Context) {
	interval := 2 * time.Second
	if p, _ := vaulttransit.LoadEnv(); p.PollInterval > 0 && p.PollInterval < interval {
		interval = p.PollInterval
	}
	s.WatchVaultConfig(ctx, interval)
}

func (s *Server) vaultLoop(stop <-chan struct{}) {
	interval := 2 * time.Second
	if p := s.svc.VaultKMS(); p != nil && p.PollInterval() > 0 {
		interval = p.PollInterval()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.probeVault()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.probeVault()
			if p := s.svc.VaultKMS(); p != nil && p.PollInterval() > 0 && p.PollInterval() != interval {
				interval = p.PollInterval()
				ticker.Reset(interval)
			}
		}
	}
}

func (s *Server) probeVault() {
	p := s.svc.VaultKMS()
	if p == nil {
		return
	}
	s.vaultMu.Lock()
	s.vaultProbeSeq++
	seq := s.vaultProbeSeq
	s.vaultMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout())
	err := p.Health(ctx)
	cancel()
	s.vaultMu.Lock()
	stale := seq != s.vaultProbeSeq
	s.vaultMu.Unlock()
	if stale || s.svc.VaultKMS() != p {
		return
	}
	code := ""
	if err != nil {
		code, _ = kmsErrorText(vaulttransit.Classify(err), p.Timeout())
	}
	up := err == nil
	if action := p.Mark(up, code); action != "" {
		s.logActivityAs("system", "", action, "kms", s.clusterNodeID())
		slog.Info("kms", "action", action, "node", s.clusterNodeID(), "code", code)
	}
	observability.SetKMSAvailable(up)
	if keys, err := s.meta.ListKMSKeys(); err == nil {
		gens := make(map[string]int64, len(keys))
		for _, key := range keys {
			gens[key.ID] = key.Generation
		}
		p.NoteGenerations(gens)
	}
	s.writeClusterHeartbeat()
	_ = s.meta.UpdateNodeKMSStatus(s.clusterNodeID(), metadata.NodeKMSStatus{
		Provider:  kms.ProviderVault,
		Available: up,
		Error:     code,
		CheckedAt: time.Now().UTC(),
	})
}

func (s *Server) kmsHealth() (string, bool) {
	if p := s.svc.VaultKMS(); p != nil {
		return kms.ProviderVault, p.Probed() && p.Available()
	}
	if s.svc.LocalKMSConfigured() {
		return kms.ProviderLocal, true
	}
	return "", true
}
