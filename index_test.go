package intervalindex

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func ids(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAddGetLenRemove(t *testing.T) {
	ix := New()
	if err := ix.Add("a", 0, 10, "alpha"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Add("b", 20, 30, "bravo"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ix.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ix.Len())
	}
	e, ok := ix.Get("a")
	if !ok || e.Start != 0 || e.End != 10 || e.Value != "alpha" {
		t.Fatalf("Get(a) = %+v, %v", e, ok)
	}
	if _, ok := ix.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	if err := ix.Remove("a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := ix.Get("a"); ok {
		t.Fatal("Get(a) after Remove should be false")
	}
	if ix.Len() != 1 {
		t.Fatalf("Len after Remove = %d, want 1", ix.Len())
	}
}

func TestErrorCases(t *testing.T) {
	ix := New()
	if err := ix.Add("", 0, 1, "x"); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("Add empty id: %v", err)
	}
	if err := ix.Add("bad", 5, 5, "x"); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("Add start==end: %v", err)
	}
	if err := ix.Add("bad", 7, 3, "x"); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("Add start>end: %v", err)
	}
	if err := ix.Add("a", 0, 10, "x"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Add("a", 1, 2, "y"); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate Add: %v", err)
	}
	if err := ix.Update("ghost", 0, 1, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update unknown: %v", err)
	}
	if err := ix.Remove("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove unknown: %v", err)
	}
	if _, err := ix.Overlap(9, 9); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("Overlap invalid: %v", err)
	}
}

func TestHalfOpenBoundaries(t *testing.T) {
	ix := New()
	mustAdd := func(id string, s, e int64) {
		if err := ix.Add(id, s, e, id); err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}
	mustAdd("a", 0, 10)
	mustAdd("b", 10, 20)

	if got := ids(ix.FindPoint(0)); !equalIDs(got, []string{"a"}) {
		t.Fatalf("FindPoint(0) = %v, want [a]", got)
	}
	// 10 is the excluded end of a and the included start of b.
	if got := ids(ix.FindPoint(10)); !equalIDs(got, []string{"b"}) {
		t.Fatalf("FindPoint(10) = %v, want [b]", got)
	}
	if got := ix.FindPoint(20); len(got) != 0 {
		t.Fatalf("FindPoint(20) = %v, want empty", ids(got))
	}
	// Touching half-open intervals do not overlap.
	hits, err := ix.Overlap(10, 15)
	if err != nil {
		t.Fatalf("Overlap: %v", err)
	}
	if got := ids(hits); !equalIDs(got, []string{"b"}) {
		t.Fatalf("Overlap(10,15) = %v, want [b]", got)
	}
}

func TestFindPointAndOverlapSorted(t *testing.T) {
	ix := New()
	// Same start, different ids: tie broken by id.
	entries := []Entry{
		{ID: "z", Start: 0, End: 100, Value: "z"},
		{ID: "a", Start: 0, End: 50, Value: "a"},
		{ID: "m", Start: 10, End: 20, Value: "m"},
		{ID: "n", Start: 30, End: 40, Value: "n"},
		{ID: "far", Start: 500, End: 600, Value: "far"},
	}
	for _, e := range entries {
		if err := ix.Add(e.ID, e.Start, e.End, e.Value); err != nil {
			t.Fatalf("Add(%s): %v", e.ID, err)
		}
	}
	if got := ids(ix.FindPoint(15)); !equalIDs(got, []string{"a", "z", "m"}) {
		t.Fatalf("FindPoint(15) = %v, want [a z m]", got)
	}
	hits, err := ix.Overlap(15, 35)
	if err != nil {
		t.Fatalf("Overlap: %v", err)
	}
	if got := ids(hits); !equalIDs(got, []string{"a", "z", "m", "n"}) {
		t.Fatalf("Overlap(15,35) = %v, want [a z m n]", got)
	}
	hits, err = ix.Overlap(200, 300)
	if err != nil {
		t.Fatalf("Overlap: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Overlap(200,300) = %v, want empty", ids(hits))
	}
}

func TestUpdateRelocates(t *testing.T) {
	ix := New()
	if err := ix.Add("a", 0, 5, "v1"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Add("b", 2, 8, "v2"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Update("a", 100, 200, "v1-new"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := ids(ix.FindPoint(3)); !equalIDs(got, []string{"b"}) {
		t.Fatalf("FindPoint(3) = %v, want [b]", got)
	}
	if got := ids(ix.FindPoint(150)); !equalIDs(got, []string{"a"}) {
		t.Fatalf("FindPoint(150) = %v, want [a]", got)
	}
	e, ok := ix.Get("a")
	if !ok || e.Start != 100 || e.End != 200 || e.Value != "v1-new" {
		t.Fatalf("Get(a) = %+v, %v", e, ok)
	}
	if ix.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ix.Len())
	}
}

// checkInvariants verifies AVL balance and the maxEnd cache at every node.
func checkInvariants(t *testing.T, n *node) (height int, maxEnd int64, count int) {
	t.Helper()
	if n == nil {
		return 0, 0, 0
	}
	lh, lm, lc := checkInvariants(t, n.left)
	rh, rm, rc := checkInvariants(t, n.right)
	if diff := lh - rh; diff < -1 || diff > 1 {
		t.Fatalf("unbalanced node %s: left=%d right=%d", n.id, lh, rh)
	}
	height = 1 + max(lh, rh)
	maxEnd = max(n.end, max(lm, rm))
	if n.height != height {
		t.Fatalf("node %s height=%d, want %d", n.id, n.height, height)
	}
	if n.maxEnd != maxEnd {
		t.Fatalf("node %s maxEnd=%d, want %d", n.id, n.maxEnd, maxEnd)
	}
	return height, maxEnd, lc + rc + 1
}

func TestMaxEndAfterRotationsAndDeletes(t *testing.T) {
	ix := New()
	const n = 64
	// Ascending starts force left rotations; interleaved second index
	// forces right rotations when inserted in reverse.
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("asc-%02d", i)
		if err := ix.Add(id, int64(i*10), int64(i*10+7), id); err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}
	for i := n - 1; i >= 0; i-- {
		id := fmt.Sprintf("desc-%02d", i)
		if err := ix.Add(id, int64(i*10+3), int64(i*10+9), id); err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}
	_, _, count := checkInvariants(t, ix.root)
	if count != 2*n {
		t.Fatalf("tree holds %d nodes, want %d", count, 2*n)
	}
	// Delete a spread of entries, then re-check structure and pruning.
	for i := 0; i < n; i += 3 {
		id := fmt.Sprintf("asc-%02d", i)
		if err := ix.Remove(id); err != nil {
			t.Fatalf("Remove(%s): %v", id, err)
		}
	}
	checkInvariants(t, ix.root)
	// Every remaining map entry must be findable via pruned queries.
	for id, e := range ix.byID {
		found := false
		for _, hit := range ix.FindPoint(e.Start) {
			if hit.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("FindPoint(%d) missed %s", e.Start, id)
		}
	}
}

func TestSnapshotAndClose(t *testing.T) {
	ix := New()
	if err := ix.Add("b", 10, 20, "vb"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Add("a", 0, 5, "va"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	snap := ix.Snapshot()
	if got := ids(snap); !equalIDs(got, []string{"a", "b"}) {
		t.Fatalf("Snapshot order = %v, want [a b]", got)
	}
	// Snapshot is independent of later mutations.
	if err := ix.Remove("a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(snap) != 2 || snap[0].ID != "a" {
		t.Fatalf("snapshot changed after Remove: %+v", snap)
	}

	ix.Close()
	ix.Close() // idempotent, must not panic
	if err := ix.Add("c", 30, 40, "vc"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Add after Close: %v", err)
	}
	if err := ix.Update("b", 0, 1, "x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Update after Close: %v", err)
	}
	if err := ix.Remove("b"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Remove after Close: %v", err)
	}
	// Reads keep working after Close.
	if _, ok := ix.Get("b"); !ok {
		t.Fatal("Get after Close should still work")
	}
	if got := ids(ix.FindPoint(15)); !equalIDs(got, []string{"b"}) {
		t.Fatalf("FindPoint after Close = %v, want [b]", got)
	}
	if _, err := ix.Overlap(0, 100); err != nil {
		t.Fatalf("Overlap after Close: %v", err)
	}
	if got := ix.Snapshot(); len(got) != 1 {
		t.Fatalf("Snapshot after Close = %v", ids(got))
	}
	if ix.Len() != 1 {
		t.Fatalf("Len after Close = %d, want 1", ix.Len())
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	ix := New()
	const workers = 4
	const ops = 50
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(2)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				id := fmt.Sprintf("w%d-%d", w, i)
				start := int64((w*ops + i) * 10)
				if err := ix.Add(id, start, start+25, id); err != nil {
					t.Errorf("Add(%s): %v", id, err)
					return
				}
				if err := ix.Update(id, start+1, start+30, id+"u"); err != nil {
					t.Errorf("Update(%s): %v", id, err)
					return
				}
				if err := ix.Remove(id); err != nil {
					t.Errorf("Remove(%s): %v", id, err)
					return
				}
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				p := int64(i * 37)
				_ = ix.FindPoint(p)
				if _, err := ix.Overlap(p, p+100); err != nil {
					t.Errorf("Overlap: %v", err)
					return
				}
				_ = ix.Snapshot()
				_ = ix.Len()
			}
		}()
	}
	wg.Wait()
	if ix.Len() != 0 {
		t.Fatalf("Len = %d, want 0 after all removes", ix.Len())
	}
	checkInvariants(t, ix.root)
}
