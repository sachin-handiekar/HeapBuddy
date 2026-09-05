package api

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
)

// Bundle is everything computed for one analyzed dump, served across the report
// endpoints. It is held in memory (no database — see CLAUDE.md), but only the
// headline report (summary, histogram, waste, top issues, leak-suspect list) is
// kept resident: those come from per-class aggregates and cost a few KB.
//
// The heavy structures — the reference graph and the dominator tree — are NOT
// built eagerly. They are needed only by the interactive views (Object Inspector,
// OQL, Dominator Tree, per-leak retention chains) and are built lazily on first
// request by re-parsing the retained dump file, then cached. A report the user
// only skims therefore holds almost nothing; the multi-GB object graph is built
// (and held) only if and when the user actually opens a view that needs it.
type Bundle struct {
	Report HeapReport
	Wasted *WastedDetail

	// Inputs retained so the interactive views can be built on demand.
	dumpPath  string        // the analyzed dump, kept until eviction (see cleanup)
	cleanup   func()        // removes dumpPath if the store owns it; nil otherwise
	graphPath string        // serialized graph cache (see SetGraphFile); "" = none
	debug     bool          // parser debug flag for lazy re-parses
	totalHeap int64         // heap size, for dominator percentages
	suspects  []LeakSuspect // leak-suspect list (cheap, from aggregates)

	// buildGate, when set, is consulted before a lazy dump re-parse. It returns
	// a release func on success, or an error (typically ErrServerBusy) when the
	// server has no capacity for another heavy build. See SetBuildGate.
	buildGate func() (release func(), err error)

	// Lazily built interactive state, guarded by mu. graphErr memoizes a failed
	// build so repeated requests don't re-parse a broken/absent dump each time.
	mu         sync.Mutex
	graph      *analysis.ReferenceGraph
	dominators *analysis.DominatorTree
	leaks      []LeakSuspectDetail
	graphErr   error

	CreatedAt time.Time
}

// close releases resources tied to the bundle — the retained dump file and the
// serialized graph cache. It is called by the store when the bundle is evicted
// or expires. Removing the graph cache is best-effort: on Windows the file
// cannot be deleted while a loaded graph still maps it, in which case the
// mapping's finalizer (or the server's boot sweep) removes it later.
func (b *Bundle) close() {
	if b.cleanup != nil {
		b.cleanup()
	}
	if b.graphPath != "" {
		_ = os.Remove(b.graphPath)
	}
}

// Store is a concurrency-safe, in-memory map of report id -> Bundle with size-
// and age-based eviction so a long-running server doesn't grow without bound.
type Store struct {
	mu      sync.Mutex
	reports map[string]*Bundle
	maxAge  time.Duration
	maxSize int
	now     func() time.Time // injectable for tests
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithMaxAge sets how long a report is retained after creation. Zero disables
// age-based eviction.
func WithMaxAge(d time.Duration) StoreOption {
	return func(s *Store) { s.maxAge = d }
}

// WithMaxSize caps the number of retained reports; the oldest are evicted first.
// Values <= 0 disable size-based eviction.
func WithMaxSize(n int) StoreOption {
	return func(s *Store) { s.maxSize = n }
}

// DefaultMaxReports caps retained reports by default. Each report keeps the whole
// object graph (and dominator tree) alive — potentially gigabytes for a large
// dump — so this is deliberately small: the bound on retained memory is roughly
// this count times the per-dump graph size. Raise it with WithMaxSize if you have
// the RAM and analyze many dumps in a session.
const DefaultMaxReports = 8

// DefaultMaxAge is how long a report is retained after creation.
const DefaultMaxAge = time.Hour

// NewStore creates a report store. Defaults: keep up to DefaultMaxReports reports
// for DefaultMaxAge.
func NewStore(opts ...StoreOption) *Store {
	s := &Store{
		reports: make(map[string]*Bundle),
		maxAge:  DefaultMaxAge,
		maxSize: DefaultMaxReports,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Put stores a bundle under a freshly generated id and returns that id. Eviction
// of expired/overflowing entries runs on each write.
func (s *Store) Put(b *Bundle) string {
	id := newID()
	s.mu.Lock()
	defer s.mu.Unlock()
	b.Report.Summary.ID = id
	b.CreatedAt = s.now() // insertion time drives expiry/eviction
	s.reports[id] = b
	s.evictLocked()
	return id
}

// Get returns the bundle for id, or (nil, false) if it is absent or expired.
func (s *Store) Get(id string) (*Bundle, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.reports[id]
	if !ok {
		return nil, false
	}
	if s.expiredLocked(b) {
		delete(s.reports, id)
		b.close()
		return nil, false
	}
	return b, true
}

func (s *Store) expiredLocked(b *Bundle) bool {
	return s.maxAge > 0 && s.now().Sub(b.CreatedAt) > s.maxAge
}

// evictLocked drops expired entries, then trims to maxSize by oldest-first,
// releasing each evicted bundle's retained dump file.
func (s *Store) evictLocked() {
	for id, b := range s.reports {
		if s.expiredLocked(b) {
			delete(s.reports, id)
			b.close()
		}
	}
	if s.maxSize <= 0 || len(s.reports) <= s.maxSize {
		return
	}
	type aged struct {
		id string
		at time.Time
	}
	all := make([]aged, 0, len(s.reports))
	for id, b := range s.reports {
		all = append(all, aged{id, b.CreatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, a := range all[:len(s.reports)-s.maxSize] {
		s.reports[a.id].close()
		delete(s.reports, a.id)
	}
}

// newID returns a short, URL-safe, unguessable report id (e.g. "rpt_9f3a2b1c").
func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is effectively impossible; fall back to time.
		return "rpt_" + hex.EncodeToString([]byte(time.Now().Format("150405")))
	}
	return "rpt_" + hex.EncodeToString(b[:])
}
