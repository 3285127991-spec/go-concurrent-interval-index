# intervalindex

A concurrency-safe in-memory interval index for Go. Stores half-open
intervals `[start, end)` (int64) keyed by unique, non-empty string IDs, each
with a string value. Pure standard library, no third-party dependencies, no
persistence, no networking.

## Design

- Core: an AVL tree ordered by `(start, id)`. Each node caches its subtree
  height and `maxEnd` (maximum interval end in the subtree); both are
  repaired on every rotation, insert and delete.
- `FindPoint` / `Overlap` prune whole subtrees via `maxEnd` instead of
  scanning all elements.
- A `map[string]*node` side index gives O(1) `Get` and locates entries for
  `Update`/`Remove`. `Update` removes the old node from the tree and
  re-inserts the new one, keeping map and tree consistent.
- A single `sync.RWMutex` makes all methods safe for concurrent use.
- Query results are ordered by `(start, id)`.

## API

```go
ix := intervalindex.New()

err := ix.Add(id string, start, end int64, value string) // ErrEmptyID / ErrInvalidRange / ErrDuplicateID / ErrClosed
err  = ix.Update(id string, start, end int64, value string) // ErrInvalidRange / ErrNotFound / ErrClosed
err  = ix.Remove(id string)                                // ErrNotFound / ErrClosed
e, err := ix.Get(id string)                                // ErrNotFound
hits := ix.FindPoint(point int64)            // entries with start <= point < end
hits, err = ix.Overlap(start, end int64)     // entries intersecting [start, end); ErrInvalidRange
snap := ix.Snapshot()                        // independent, ordered copy of all entries
n    := ix.Len()
ix.Close() // idempotent; writes fail with ErrClosed, reads keep working
```

Errors are sentinel values (`ErrDuplicateID`, `ErrNotFound`, ...); match them
with `errors.Is`.

## CLI demo

```sh
go run ./cmd/intervalcli
```

Demonstrates Add, FindPoint, Overlap, Update, Remove, and reads after Close.

## Tests

```sh
go test -race ./...
```

Covers basic CRUD and error cases, half-open boundary semantics, point and
overlap queries, update relocation, AVL/maxEnd invariants after rotations and
deletes, snapshot/Close behavior, and concurrent read/write access.
