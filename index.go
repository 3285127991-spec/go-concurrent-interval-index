// Package intervalindex provides a concurrency-safe in-memory index of
// half-open int64 intervals [start, end) keyed by unique string ids.
//
// The core is an AVL tree ordered by (start, id) with a per-node subtree
// maxEnd cache, so point and overlap queries prune subtrees instead of
// scanning every element.
package intervalindex

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrEmptyID         = errors.New("intervalindex: id must be non-empty")
	ErrInvalidInterval = errors.New("intervalindex: invalid interval, need start < end")
	ErrDuplicateID     = errors.New("intervalindex: id already exists")
	ErrNotFound        = errors.New("intervalindex: id not found")
	ErrClosed          = errors.New("intervalindex: index is closed for writes")
)

// Entry is one stored interval. Intervals are half-open: [Start, End).
type Entry struct {
	ID    string
	Start int64
	End   int64
	Value string
}

// Index maps unique ids to half-open intervals and answers point and
// overlap queries. It is safe for concurrent use.
type Index struct {
	mu     sync.RWMutex
	root   *node
	byID   map[string]Entry
	closed bool
}

// New returns an empty, ready-to-use Index.
func New() *Index {
	return &Index{byID: make(map[string]Entry)}
}

func validate(id string, start, end int64) error {
	if id == "" {
		return ErrEmptyID
	}
	if start >= end {
		return fmt.Errorf("%w: [%d, %d)", ErrInvalidInterval, start, end)
	}
	return nil
}

// Add inserts a new entry. It fails if the id is empty, the interval is
// invalid, the id already exists, or the index is closed.
func (ix *Index) Add(id string, start, end int64, value string) error {
	if err := validate(id, start, end); err != nil {
		return err
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	if _, ok := ix.byID[id]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicateID, id)
	}
	ix.root = insertNode(ix.root, id, start, end, value)
	ix.byID[id] = Entry{ID: id, Start: start, End: end, Value: value}
	return nil
}

// Update replaces the interval and value of an existing id. The old tree
// node is removed and a new one inserted, keeping the id index and the
// tree consistent. It fails for unknown ids, invalid input, or a closed
// index.
func (ix *Index) Update(id string, start, end int64, value string) error {
	if err := validate(id, start, end); err != nil {
		return err
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	old, ok := ix.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	ix.root = deleteNode(ix.root, old.Start, old.ID)
	ix.root = insertNode(ix.root, id, start, end, value)
	ix.byID[id] = Entry{ID: id, Start: start, End: end, Value: value}
	return nil
}

// Remove deletes the entry for id. It fails for unknown ids or a closed
// index.
func (ix *Index) Remove(id string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	old, ok := ix.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	ix.root = deleteNode(ix.root, old.Start, old.ID)
	delete(ix.byID, id)
	return nil
}

// Get returns the entry for id, or false if it does not exist.
func (ix *Index) Get(id string) (Entry, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	e, ok := ix.byID[id]
	return e, ok
}

// FindPoint returns all entries whose interval contains point
// (start <= point < end), sorted by (start, id).
func (ix *Index) FindPoint(point int64) []Entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Entry
	findPoint(ix.root, point, &out)
	return out
}

// Overlap returns all entries intersecting the half-open query interval
// [start, end), sorted by (start, id). It fails if start >= end.
func (ix *Index) Overlap(start, end int64) ([]Entry, error) {
	if start >= end {
		return nil, fmt.Errorf("%w: [%d, %d)", ErrInvalidInterval, start, end)
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Entry
	overlap(ix.root, start, end, &out)
	return out, nil
}

// Snapshot returns an independent, sorted-by-(start, id) copy of all
// entries. Mutating the index afterwards does not affect the snapshot.
func (ix *Index) Snapshot() []Entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make([]Entry, 0, len(ix.byID))
	inOrder(ix.root, &out)
	return out
}

// Len returns the number of stored entries.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.byID)
}

// Close marks the index closed: Add, Update and Remove start failing with
// ErrClosed, while Get, FindPoint, Overlap, Snapshot and Len keep working.
// Close is idempotent.
func (ix *Index) Close() {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.closed = true
}
