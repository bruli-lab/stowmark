package snapshot

type StoredObject struct {
	hash string
	size int64
}

func (s StoredObject) Hash() string {
	return s.hash
}

func (s StoredObject) Size() int64 {
	return s.size
}

func NewStoredObject(hash string, size int64) StoredObject {
	return StoredObject{hash: hash, size: size}
}

func StoredHashes(objects []StoredObject) []string {
	hashes := make([]string, len(objects))
	for i, object := range objects {
		hashes[i] = object.hash
	}
	return hashes
}
