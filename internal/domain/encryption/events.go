package encryption

import (
	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const (
	SymmetricKeyRotatedEventName = "encryption.symmetric_key_rotated"
	KeyRewrappedEventName        = "encryption.key_rewrapped"
)

type SymmetricKeyRotated struct {
	event.BasicEvent
	generation     uint64
	rekeyedObjects int
}

func (s *SymmetricKeyRotated) RepositoryID() string {
	return s.AggregateRootID()
}

func (s *SymmetricKeyRotated) Generation() uint64 {
	return s.generation
}

func (s *SymmetricKeyRotated) RekeyedObjects() int {
	return s.rekeyedObjects
}

func (e *EncryptionConfig) SymmetricKeyRotated(repositoryID string, rekeyedObjects int) *SymmetricKeyRotated {
	return &SymmetricKeyRotated{
		BasicEvent:     event.NewBasicEvent(SymmetricKeyRotatedEventName, uuid.New(), repositoryID),
		generation:     e.Generation(),
		rekeyedObjects: rekeyedObjects,
	}
}

type KeyRewrapped struct {
	event.BasicEvent
	publicKeyFingerprint string
}

func (k *KeyRewrapped) RepositoryID() string {
	return k.AggregateRootID()
}

func (k *KeyRewrapped) PublicKeyFingerprint() string {
	return k.publicKeyFingerprint
}

func (e *EncryptionConfig) KeyRewrapped(repositoryID string) *KeyRewrapped {
	return &KeyRewrapped{
		BasicEvent:           event.NewBasicEvent(KeyRewrappedEventName, uuid.New(), repositoryID),
		publicKeyFingerprint: e.PublicKeyFingerprint(),
	}
}
