package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

// SessionIncarnationStateKey moves with the Session's canonical State during
// backend migration, while a recreated or expired Session receives a new UUID.
const SessionIncarnationStateKey = "platform:session_incarnation_id"

var ErrSessionIncarnation = errors.New("session incarnation is unavailable or protected")

// SessionInvocationLeaseKey is shared by Worker, Summary, and initialization.
func SessionInvocationLeaseKey(tenantID string, key session.Key) string {
	digest := sha256.Sum256([]byte(tenantID + "\x00" + key.AppName + "\x00" + key.UserID + "\x00" + key.SessionID))
	return "session-invocation:" + hex.EncodeToString(digest[:])
}

type sessionLeaseBindingKey struct{}

type sessionLeaseBinding struct {
	key   session.Key
	lease *Lease
	mu    sync.Mutex
	id    string
}

// ContextWithSessionLease lets strict fenced Session operations initialize a
// legacy Session only while the complete invocation owns its renewable lease.
func ContextWithSessionLease(ctx context.Context, key session.Key, lease *Lease) context.Context {
	return context.WithValue(ctx, sessionLeaseBindingKey{}, &sessionLeaseBinding{key: key, lease: lease})
}

// SessionIncarnationFromContext retains the incarnation observed by Runner even
// if the final receipt's bounded Session read is unavailable.
func SessionIncarnationFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	binding, _ := ctx.Value(sessionLeaseBindingKey{}).(*sessionLeaseBinding)
	if binding == nil {
		return ""
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	return binding.id
}

func SessionIncarnationID(value *session.Session) (string, error) {
	if value == nil {
		return "", nil
	}
	raw, found := value.GetState(SessionIncarnationStateKey)
	if !found {
		return "", nil
	}
	id := string(raw)
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil || parsed.String() != id {
		return "", ErrSessionIncarnation
	}
	return id, nil
}

func protectedSessionState(state session.StateMap) error {
	if _, found := state[SessionIncarnationStateKey]; found {
		return ErrSessionIncarnation
	}
	return nil
}

// Called only inside the strict Session service's execution fence.
func (s *FencedSessionService) bindSessionIncarnation(ctx context.Context, key session.Key, value *session.Session) (*session.Session, error) {
	if value == nil || !s.scope.strict {
		return value, nil
	}
	id, err := SessionIncarnationID(value)
	if err != nil {
		return nil, err
	}
	binding, _ := ctx.Value(sessionLeaseBindingKey{}).(*sessionLeaseBinding)
	if binding == nil {
		return value, nil
	}
	if binding.key != key || binding.lease == nil || binding.lease.lock == nil || binding.lease.manager == nil ||
		binding.lease.lock.Key != SessionInvocationLeaseKey(s.scope.tenantID, key) {
		return nil, ErrSessionIncarnation
	}
	select {
	case <-binding.lease.Done():
		return nil, ErrStaleWriter
	default:
	}
	if binding.lease.Err() != nil || !binding.lease.manager.ValidateLock(ctx, binding.lease.lock) {
		return nil, ErrStaleWriter
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if id == "" {
		// Another operation in this invocation may have bound the initially
		// unmarked value while this call waited. Never overwrite that binding.
		current, readErr := s.inner.GetSession(ctx, key, session.WithEventNum(1))
		if readErr != nil {
			return nil, readErr
		}
		if current == nil || !current.CreatedAt.Equal(value.CreatedAt) {
			return nil, ErrSessionIncarnation
		}
		id, err = SessionIncarnationID(current)
		if err != nil {
			return nil, err
		}
	}
	if binding.id != "" && binding.id != id {
		return nil, ErrSessionIncarnation
	}
	if id == "" {
		id = uuid.NewString()
		if err := s.inner.UpdateSessionState(ctx, key, session.StateMap{SessionIncarnationStateKey: []byte(id)}); err != nil {
			return nil, err
		}
	}
	value = value.Clone()
	value.SetState(SessionIncarnationStateKey, []byte(id))
	binding.id = id
	return value, nil
}
