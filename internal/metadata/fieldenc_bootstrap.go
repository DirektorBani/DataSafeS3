package metadata

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

// BootstrapFieldEncryption syncs the KEK registry with env and marks retired keys on the service.
func BootstrapFieldEncryption(ctx context.Context, svc *fieldenc.Service, store MetadataStore) error {
	if svc == nil || !svc.Enabled() {
		return nil
	}
	keys, err := store.ListEncryptionKeys(ctx)
	if err != nil {
		return fmt.Errorf("fieldenc: list registry: %w", err)
	}
	retired := make(map[string]bool)
	for _, k := range keys {
		if k.RetiredAt != nil {
			retired[k.KEKID] = true
		}
	}
	svc.MarkRetiredKEKs(retired)

	if len(keys) == 0 {
		rec := EncryptionKeyRecord{
			KEKID:     svc.ActiveKEKID(),
			Algorithm: EncryptionAlgorithmX25519,
			PublicKey: svc.ActivePublicKey(),
			IsActive:  true,
			CreatedAt: time.Now().UTC(),
		}
		if err := store.InsertEncryptionKey(ctx, rec); err != nil {
			return fmt.Errorf("fieldenc: bootstrap registry: %w", err)
		}
		return nil
	}
	active, err := store.GetActiveEncryptionKey(ctx)
	if err != nil {
		return fmt.Errorf("fieldenc: registry has keys but no active KEK: %w", err)
	}
	if active.KEKID != svc.ActiveKEKID() {
		return fmt.Errorf("fieldenc: STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID %q does not match registry active %q",
			svc.ActiveKEKID(), active.KEKID)
	}
	if !bytes.Equal(active.PublicKey, svc.ActivePublicKey()) {
		return fmt.Errorf("fieldenc: env KEK public key does not match registry for %q", active.KEKID)
	}
	return nil
}

// AttachFieldEncryption wires the service into bolt or postgres metadata stores.
func AttachFieldEncryption(store MetadataStore, svc *fieldenc.Service) {
	switch s := store.(type) {
	case *Store:
		s.fieldenc = svc
	case fieldEncPostgresSetter:
		s.SetFieldEncryption(svc)
	}
}

type fieldEncPostgresSetter interface {
	SetFieldEncryption(*fieldenc.Service)
}
