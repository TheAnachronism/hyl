package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"
)

// handshakeTTL bounds how long an OAuth handshake cookie stays valid; a user
// has this long to complete the provider round trip.
const handshakeTTL = 15 * time.Minute

// handshakeStore implements gorilla's sessions.Store for goth's OAuth
// handshake. Only the session *id* travels in the cookie; the provider state
// lives in process memory for a few minutes, so the handshake cannot be forged
// and nothing user-visible is ever stored client-side.
//
// The trade-off is that an OAuth handshake must land on the process that
// started it. hyl runs as a single binary against a single SQLite file, so that
// is always true; a future multi-replica deployment would need this moved into
// the database.
type handshakeStore struct {
	mu       sync.Mutex
	entries  map[string]handshakeEntry
	ttl      time.Duration
	secure   bool
	sameSite http.SameSite
}

type handshakeEntry struct {
	values  map[any]any
	expires time.Time
}

func newHandshakeStore(secure bool) *handshakeStore {
	return &handshakeStore{
		entries:  make(map[string]handshakeEntry),
		ttl:      handshakeTTL,
		secure:   secure,
		sameSite: http.SameSiteLaxMode,
	}
}

func (s *handshakeStore) options() *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   int(s.ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: s.sameSite,
	}
}

func (s *handshakeStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	sess := sessions.NewSession(s, name)
	sess.Options = s.options()

	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value == "" {
		return sess, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[cookie.Value]
	if !ok || !entry.expires.After(time.Now()) {
		delete(s.entries, cookie.Value)
		return sess, nil
	}
	sess.ID = cookie.Value
	sess.Values = entry.values
	return sess, nil
}

func (s *handshakeStore) New(r *http.Request, name string) (*sessions.Session, error) {
	return s.Get(r, name)
}

func (s *handshakeStore) Save(_ *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	opts := sess.Options
	if opts == nil {
		opts = s.options()
	}

	if opts.MaxAge < 0 {
		s.mu.Lock()
		delete(s.entries, sess.ID)
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:     sess.Name(),
			Value:    "",
			Path:     opts.Path,
			MaxAge:   -1,
			HttpOnly: opts.HttpOnly,
			Secure:   opts.Secure,
			SameSite: opts.SameSite,
		})
		return nil
	}

	id := sess.ID
	if id == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		id = base64.RawURLEncoding.EncodeToString(buf)
	}

	values := make(map[any]any, len(sess.Values))
	for k, v := range sess.Values {
		values[k] = v
	}

	s.mu.Lock()
	s.gcLocked()
	s.entries[id] = handshakeEntry{values: values, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()

	sess.ID = id
	sess.IsNew = false
	http.SetCookie(w, &http.Cookie{
		Name:     sess.Name(),
		Value:    id,
		Path:     opts.Path,
		MaxAge:   opts.MaxAge,
		HttpOnly: opts.HttpOnly,
		Secure:   opts.Secure,
		SameSite: opts.SameSite,
	})
	return nil
}

// gcLocked drops expired handshakes; it runs on every save, which is rare
// enough that a full scan is cheaper than tracking a queue.
func (s *handshakeStore) gcLocked() {
	now := time.Now()
	for id, entry := range s.entries {
		if !entry.expires.After(now) {
			delete(s.entries, id)
		}
	}
}
