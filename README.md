# intervalindex

A concurrency-safe, in-memory interval index for Go. It stores half-open
int64 intervals `[start, end)` keyed by unique, non-empty string ids, each
with a string value. Pure standard library; no third-party dependencies,
no persistence, no network.

## Design

- Core: an AVL tree (self-balancing BST) ordered by `(start, id)`.
- Every node caches its subtree `maxEnd`; point and overlap queries prune
  subtrees whose `maxEnd` cannot intersect the query, so lookups never
  scan the whole set. Rotations, deletes and updates recompute
  `height`/`maxEnd` on the way back up.
- A `map[string]Entry` id index mirrors the tree; `Update` removes the
  old tree node and reinserts at the new position, keeping both consistent.
- Concurrency: one `sync.RWMutex` — writes are exclusive, reads are
  parallel.

## API

| Method | Behavior |
| --- | --- |
| `New()` | empty index |
| `Add(id, start, end, value) error` | fails on empty id, `start >= end`, duplicate id, closed index |
| `Update(id, start, end, value) error` | repositions the interval; fails on unknown id / invalid input / closed |
| `Remove(id) error` | fails on unknown id / closed |
| `Get(id) (Entry, bool)` | point lookup by id |
| `FindPoint(point) []Entry` | all entries with `start <= point < end`, sorted by `(start, id)` |
| `Overlap(start, end) ([]Entry, error)` | all entries intersecting `[start, end)`, sorted by `(start, id)` |
| `Snapshot() []Entry` | independent sorted copy of everything |
| `Len() int` | number of entries |
| `Close()` | idempotent; writes fail with `ErrClosed`, reads keep working |

Errors are sentinels (`ErrEmptyID`, `ErrInvalidInterval`,
`ErrDuplicateID`, `ErrNotFound`, `ErrClosed`) wrapped with context —
match them with `errors.Is`.

## Layout

- `tree.go` — AVL tree with `maxEnd` augmentation and pruned queries
- `index.go` — public `Index` API, locking, validation
- `cmd/intervalindex/main.go` — CLI demo (add, point/overlap query,
  update, remove, reads after close)
- `index_test.go` — unit tests

## Run

```sh
go test -race ./...
go run ./cmd/intervalindex
```
