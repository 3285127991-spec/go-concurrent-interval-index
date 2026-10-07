// Package intervalindex provides a concurrency-safe in-memory index of
// half-open int64 intervals [start, end) keyed by unique string IDs.
//
// The core is an AVL tree ordered by (start, id). Every node maintains the
// subtree height and the maximum end value (maxEnd) of its subtree, so point
// and overlap queries prune whole subtrees instead of scanning everything.
package intervalindex

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

// Entry is a single indexed interval [Start, End) with its ID and value.
type Entry struct {
	ID    string
	Start int64
	End   int64
	Value string
}

var (
	ErrEmptyID      = errors.New("intervalindex: id must not be empty")
	ErrInvalidRange = errors.New("intervalindex: invalid interval: start must be < end")
	ErrDuplicateID  = errors.New("intervalindex: id already exists")
	ErrNotFound     = errors.New("intervalindex: id not found")
	ErrClosed       = errors.New("intervalindex: index is closed for writes")
)

type node struct {
	id     string
	start  int64
	end    int64
	value  string
	left   *node
	right  *node
	height int
	maxEnd int64
}

// Index is a concurrent interval index. Create one with New.
type Index struct {
	mu     sync.RWMutex
	root   *node
	byID   map[string]*node
	closed bool
}

// New returns an empty, ready-to-use Index.
func New() *Index {
	return &Index{byID: make(map[string]*node)}
}

// Add inserts a new entry. It fails with ErrEmptyID, ErrInvalidRange,
// ErrDuplicateID or ErrClosed.
func (ix *Index) Add(id string, start, end int64, value string) error {
	if id == "" {
		return ErrEmptyID
	}
	if start >= end {
		return fmt.Errorf("%w: [%d, %d)", ErrInvalidRange, start, end)
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	if _, ok := ix.byID[id]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicateID, id)
	}
	nn := &node{id: id, start: start, end: end, value: value, height: 1, maxEnd: end}
	ix.root = insert(ix.root, nn)
	ix.byID[id] = nn
	return nil
}

// Update replaces the interval and value of an existing id. The old node is
// removed from the tree and a new one is re-inserted so the id index and the
// tree stay consistent. It fails with ErrInvalidRange, ErrNotFound or
// ErrClosed.
func (ix *Index) Update(id string, start, end int64, value string) error {
	if start >= end {
		return fmt.Errorf("%w: [%d, %d)", ErrInvalidRange, start, end)
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	n, ok := ix.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	ix.root = deleteNode(ix.root, n.start, n.id)
	nn := &node{id: id, start: start, end: end, value: value, height: 1, maxEnd: end}
	ix.root = insert(ix.root, nn)
	ix.byID[id] = nn
	return nil
}

// Remove deletes the entry with the given id. It fails with ErrNotFound or
// ErrClosed.
func (ix *Index) Remove(id string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	n, ok := ix.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	ix.root = deleteNode(ix.root, n.start, n.id)
	delete(ix.byID, id)
	return nil
}

// Get returns the entry for id, or ErrNotFound.
func (ix *Index) Get(id string) (Entry, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	n, ok := ix.byID[id]
	if !ok {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return entryOf(n), nil
}

// FindPoint returns all entries whose interval contains point
// (start <= point < end), ordered by (start, id).
func (ix *Index) FindPoint(point int64) []Entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Entry
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			return
		}
		// Prune: no interval in the left subtree can contain point.
		if n.left != nil && n.left.maxEnd > point {
			walk(n.left)
		}
		if n.start <= point && point < n.end {
			out = append(out, entryOf(n))
		}
		// Prune: everything on the right starts at or after n.start.
		if n.start <= point {
			walk(n.right)
		}
	}
	walk(ix.root)
	return out
}

// Overlap returns all entries intersecting the half-open query interval
// [start, end), ordered by (start, id). It fails with ErrInvalidRange.
func (ix *Index) Overlap(start, end int64) ([]Entry, error) {
	if start >= end {
		return nil, fmt.Errorf("%w: [%d, %d)", ErrInvalidRange, start, end)
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Entry
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			return
		}
		// Prune: no interval in the left subtree reaches into the query.
		if n.left != nil && n.left.maxEnd > start {
			walk(n.left)
		}
		if n.start < end && n.end > start {
			out = append(out, entryOf(n))
		}
		// Prune: everything on the right starts at or after n.start.
		if n.start < end {
			walk(n.right)
		}
	}
	walk(ix.root)
	return out, nil
}

// Snapshot returns an independent, ordered copy of all entries.
func (ix *Index) Snapshot() []Entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make([]Entry, 0, len(ix.byID))
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			return
		}
		walk(n.left)
		out = append(out, entryOf(n))
		walk(n.right)
	}
	walk(ix.root)
	return out
}

// Len returns the number of entries.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.byID)
}

// Close marks the index closed for writes. Add, Update and Remove return
// ErrClosed afterwards, while all read operations keep working.
// Close is idempotent.
func (ix *Index) Close() {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.closed = true
}

func entryOf(n *node) Entry {
	return Entry{ID: n.id, Start: n.start, End: n.end, Value: n.value}
}

// less reports whether key (start, id) sorts before n.
func less(start int64, id string, n *node) bool {
	return start < n.start || (start == n.start && id < n.id)
}

// greater reports whether key (start, id) sorts after n.
func greater(start int64, id string, n *node) bool {
	return start > n.start || (start == n.start && id > n.id)
}

func heightOf(n *node) int {
	if n == nil {
		return 0
	}
	return n.height
}

func maxEndOf(n *node) int64 {
	if n == nil {
		return math.MinInt64
	}
	return n.maxEnd
}

// recalc restores height and maxEnd from the (already fixed) children.
func (n *node) recalc() {
	n.height = 1 + max(heightOf(n.left), heightOf(n.right))
	n.maxEnd = max(n.end, maxEndOf(n.left), maxEndOf(n.right))
}

func rotateLeft(n *node) *node {
	r := n.right
	n.right = r.left
	r.left = n
	n.recalc()
	r.recalc()
	return r
}

func rotateRight(n *node) *node {
	l := n.left
	n.left = l.right
	l.right = n
	n.recalc()
	l.recalc()
	return l
}

// rebalance fixes height/maxEnd and restores the AVL height invariant.
func rebalance(n *node) *node {
	n.recalc()
	switch balance := heightOf(n.left) - heightOf(n.right); {
	case balance > 1:
		if heightOf(n.left.left) < heightOf(n.left.right) {
			n.left = rotateLeft(n.left)
		}
		return rotateRight(n)
	case balance < -1:
		if heightOf(n.right.right) < heightOf(n.right.left) {
			n.right = rotateRight(n.right)
		}
		return rotateLeft(n)
	default:
		return n
	}
}

func insert(n, nn *node) *node {
	if n == nil {
		return nn
	}
	if less(nn.start, nn.id, n) {
		n.left = insert(n.left, nn)
	} else {
		n.right = insert(n.right, nn)
	}
	return rebalance(n)
}

func deleteNode(n *node, start int64, id string) *node {
	if n == nil {
		return nil
	}
	switch {
	case less(start, id, n):
		n.left = deleteNode(n.left, start, id)
	case greater(start, id, n):
		n.right = deleteNode(n.right, start, id)
	default:
		if n.left == nil {
			return n.right
		}
		if n.right == nil {
			return n.left
		}
		succ := n.right
		for succ.left != nil {
			succ = succ.left
		}
		n.id, n.start, n.end, n.value = succ.id, succ.start, succ.end, succ.value
		n.right = deleteNode(n.right, succ.start, succ.id)
	}
	return rebalance(n)
}
