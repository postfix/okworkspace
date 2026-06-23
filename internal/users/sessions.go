package users

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/postfix/okworkspace/internal/auth"
)

// sessionUserID decodes one persisted SCS session blob and returns the
// authenticated user id stored under auth.SessionUserIDKey, or 0 when the blob
// does not decode or carries no user id. SCS's default GobCodec encodes the
// session as {Deadline time.Time, Values map[string]interface{}}; the login
// handler stores the user id (an int64) under SessionUserIDKey. We mirror that
// layout here rather than depend on an SCS export, keeping per-user session
// revocation (WR-02) self-contained.
func sessionUserID(data []byte) int64 {
	aux := &struct {
		Deadline time.Time
		Values   map[string]interface{}
	}{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(aux); err != nil {
		return 0
	}
	v, ok := aux.Values[auth.SessionUserIDKey]
	if !ok {
		return 0
	}
	switch id := v.(type) {
	case int64:
		return id
	case int:
		return int64(id)
	default:
		return 0
	}
}
