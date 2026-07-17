package suggest

import (
	"context"
	"sync"
	"time"
)

// entry is one key's cached state. seq is a per-key monotonic counter, not a
// timestamp: it orders concurrent fetches by the moment each one *started*
// under the lock, so a slow fetch that started earlier can be recognized and
// discarded even if it finishes after a newer one. value/storedAt only mean
// anything once fetched is true.
type entry[V any] struct {
	value    V
	storedAt time.Time
	seq      uint64 // seq of the fetch whose result is currently committed
	next     uint64 // next seq to hand out; also the newest fetch in flight
	fetched  bool
}

// Cached memoizes a slow, keyed lookup for a TTL window so repeated queries
// reuse the last good result instead of re-hitting the source.
type Cached[K comparable, V any] struct {
	// mu guards entries. It is deliberately NOT held across the source call:
	// source is the slow thing (a Jira round-trip), and holding the lock for
	// its duration would serialize every fetch — a create form typing into
	// two fields would block one lookup behind the other. We lock only to
	// read/allocate sequence state and, later, to commit.
	mu      sync.Mutex
	entries map[K]*entry[V]

	ttl    time.Duration
	source func(context.Context, K) (V, error)
	now    func() time.Time
}

// New builds a Cached over source with the given TTL. now supplies the clock;
// pass nil to default to time.Now (tests inject a fake clock).
func New[K comparable, V any](ttl time.Duration, source func(context.Context, K) (V, error), now func() time.Time) *Cached[K, V] {
	if now == nil {
		now = time.Now
	}
	return &Cached[K, V]{
		entries: make(map[K]*entry[V]),
		ttl:     ttl,
		source:  source,
		now:     now,
	}
}

// Get returns the value for key: a cached value still within its TTL, or a
// fresh source call whose result is cached before return.
func (c *Cached[K, V]) Get(ctx context.Context, key K) (V, error) {
	// Phase 1 (locked): serve a fresh cache hit, or reserve a sequence for
	// the fetch we are about to run. We compute expiry off c.now, never
	// time.Now directly, so an injected clock fully controls TTL.
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		e = &entry[V]{}
		c.entries[key] = e
	}
	if e.fetched && c.now().Before(e.storedAt.Add(c.ttl)) {
		v := e.value
		c.mu.Unlock()
		return v, nil
	}
	// Reserve a sequence for this fetch. next always holds the highest
	// sequence handed out, so a later Get on the same key gets a strictly
	// larger seq and wins the store race below.
	e.next++
	seq := e.next
	c.mu.Unlock()

	// Phase 2 (unlocked): the slow source call. Concurrent Gets for this key
	// run their source calls in parallel here, each with its own seq.
	v, err := c.source(ctx, key)
	if err != nil {
		// Do not cache failures: the next Get must retry rather than serve a
		// stale error for a whole TTL window. seq is simply abandoned.
		return v, err
	}

	// Phase 3 (locked): commit, but only if we are the newest fetch to reach
	// this point. Without this guard, an earlier slow fetch finishing after a
	// newer fast fetch would clobber the fresher value — the last *writer*
	// would win instead of the last *reader*. Comparing against the committed
	// seq (not next) means we still commit when no newer fetch has committed
	// yet, even if a newer fetch is mid-flight.
	c.mu.Lock()
	if seq > e.seq {
		e.value = v
		e.storedAt = c.now()
		e.seq = seq
		e.fetched = true
	}
	// Whether we committed or discarded, return the value this call fetched:
	// the caller asked for a result and got a valid one; only the shared
	// cache slot is subject to the newest-wins rule.
	c.mu.Unlock()
	return v, nil
}
