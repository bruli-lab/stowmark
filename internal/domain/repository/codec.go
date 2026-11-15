package repository

import "slices"

type Codec struct {
	version     FormatVersion
	storage     *codecStorage
	dataKey     []byte
	generation  uint64
	metadataKey []byte
}

type codecStorage struct {
	compression *Compression
	objectLock  *ObjectLock
}

func (s *codecStorage) copy() *codecStorage {
	if s == nil {
		return &codecStorage{}
	}
	copied := *s
	return &copied
}

func (c Codec) ObjectLock() *ObjectLock {
	if c.storage == nil {
		return nil
	}
	return c.storage.objectLock
}

func (c Codec) WithObjectLock(lock *ObjectLock) Codec {
	c.storage = c.storage.copy()
	c.storage.objectLock = lock
	return c
}

func (c Codec) Version() FormatVersion {
	return c.version
}

func (c Codec) Compression() *Compression {
	if c.storage == nil {
		return nil
	}
	return c.storage.compression
}

func (c Codec) DataKey() []byte {
	return c.dataKey
}

func (c Codec) Generation() uint64 {
	return c.generation
}

func (c Codec) MetadataKey() []byte {
	return c.metadataKey
}

func (c Codec) EncryptsData() bool {
	return len(c.dataKey) > 0
}

func (c Codec) EncryptsMetadata() bool {
	return c.version.ContentAddressed() && len(c.metadataKey) > 0
}

func (c Codec) ContentAddressed() bool {
	return c.version.ContentAddressed()
}

func (c Codec) BuffersEncryptedObjects() bool {
	return c.EncryptsData() && !c.ContentAddressed()
}

func (c Codec) WithCompression(compression *Compression) Codec {
	c.storage = c.storage.copy()
	c.storage.compression = compression
	return c
}

func (c Codec) WithDataKey(key []byte, generation uint64) Codec {
	c.dataKey = slices.Clone(key)
	c.generation = generation
	return c
}

func (c Codec) WithMetadataKey(key []byte) Codec {
	c.metadataKey = slices.Clone(key)
	return c
}

func NewCodec(version FormatVersion, compression *Compression) Codec {
	return Codec{version: version, storage: &codecStorage{compression: compression}}
}

func (c Config) Codec() Codec {
	return NewCodec(c.formatVersion, c.compression).WithObjectLock(c.objectLock)
}
