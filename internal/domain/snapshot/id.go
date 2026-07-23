package snapshot

import (
	"time"

	"github.com/google/uuid"
)

func newID() string {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	random := uuid.NewString()[:8]

	return timestamp + "-" + random
}
