package intervalindex

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
)

func idsOf(entries []Entry) []string {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func assertIDs(t *testing.T, entries []Entry, want []string) {
	t.Helper()
	if got := idsOf(entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("got ids %v, want %v", got, want)
	}
}

// checkInvariants verifies AVL balance, height and maxEnd for every node.
func checkInvariants(t *testing.T, n *node) (height int, maxEnd int64) {
	t.Helper()
	if n == nil {
		return 0, math.MinInt64
	}
	lh, lm := checkInvariants(t, n.left)
	rh, rm := checkInvariants(t, n.right)
	if d := lh - rh; d < -1 || d > 1 {
		t.Errorf("node %q: unbalanced (left height %d, right height %d)", n.id, lh, rh)
	}
	height = 1 + max(lh, rh)
	if n.height != height {
		t.Errorf("node %q: height = %d, want %d", n.id, n.height, height)
	}
	maxEnd = max(n.end, lm, rm)
	if n.maxEnd != maxEnd {
		t.Errorf("node %q: maxEnd = %d, want %d", n.id, n.maxEnd, maxEnd)
	}
	return height, maxEnd
}

func TestAddGetRemoveAndErrors(t *testing.T) {
	ix := New()

	if err := ix.Add("a", 0, 10, "alpha"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := ix.Add("a", 1, 2, "dup"); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate Add: got %v, want ErrDuplicateID", err)
	}
	if err := ix.Add("", 0, 1, "empty"); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("empty id Add: got %v, want ErrEmptyID", err)
	}
	if err := ix.Add("bad", 10, 10, "x"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("start==end Add: got %v, want ErrInvalidRange", err)
	}
	if err := ix.Add("bad2", 10, 5, "x"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("start>end Add: got %v, want ErrInvalidRange", err)
	}

	e, err := ix.Get("a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e != (Entry{ID: "a", Start: 0, End: 10, Value: "alpha"}) {
		t.Fatalf("Get returned %+v", e)
	}
	if _, err := ix.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing: got %v, want ErrNotFound", err)
	}
	if ix.Len() != 1 {
		t.Fatalf("Len = %d, want 1", ix.Len())
	}

	if err := ix.Remove("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove missing: got %v, want ErrNotFound", err)
	}
	if err := ix.Update("missing", 0, 1, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update missing: got %v, want ErrNotFound", err)
	}
	if err := ix.Remove("a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ix.Len() != 0 {
		t.Fatalf("Len after Remove = %d, want 0", ix.Len())
	}
	if _, err := ix.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Remove: got %v, want ErrNotFound", err)
	}
}

func TestHalfOpenBoundaries(t *testing.T) {
	ix := New()
	if err := ix.Add("a", 10, 20, "A"); err != nil {
		t.Fatal(err)
	}

	assertIDs(t, ix.FindPoint(10), []string{"a"}) // start inclusive
	assertIDs(t, ix.FindPoint(19), []string{"a"})
	assertIDs(t, ix.FindPoint(20), nil) // end exclusive
	assertIDs(t, ix.FindPoint(9), nil)

	mustOverlap := func(start, end int64) []Entry {
		t.Helper()
		got, err := ix.Overlap(start, end)
		if err != nil {
			t.Fatalf("Overlap(%d, %d): %v", start, end, err)
		}
		return got
	}
	assertIDs(t, mustOverlap(0, 10), nil)  // touching at start: no overlap
	assertIDs(t, mustOverlap(20, 30), nil) // touching at end: no overlap
	assertIDs(t, mustOverlap(19, 20), []string{"a"})
	assertIDs(t, mustOverlap(10, 11), []string{"a"})

	if _, err := ix.Overlap(5, 5); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Overlap(5, 5): got %v, want ErrInvalidRange", err)
	}
	if _, err := ix.Overlap(30, 20); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Overlap(30, 20): got %v, want ErrInvalidRange", err)
	}
}

func TestFindPoint(t *testing.T) {
	ix := New()
	for _, e := range []Entry{
		{ID: "a", Start: 0, End: 10, Value: "A"},
		{ID: "b", Start: 5, End: 15, Value: "B"},
		{ID: "c", Start: 5, End: 8, Value: "C"},
		{ID: "d", Start: 20, End: 30, Value: "D"},
	} {
		if err := ix.Add(e.ID, e.Start, e.End, e.Value); err != nil {
			t.Fatal(err)
		}
	}

	assertIDs(t, ix.FindPoint(5), []string{"a", "b", "c"}) // sorted by (start, id)
	assertIDs(t, ix.FindPoint(8), []string{"a", "b"})
	assertIDs(t, ix.FindPoint(12), []string{"b"})
	assertIDs(t, ix.FindPoint(25), []string{"d"})
	assertIDs(t, ix.FindPoint(50), nil)
	assertIDs(t, ix.FindPoint(-1), nil)
}

func TestOverlap(t *testing.T) {
	ix := New()
	for _, e := range []Entry{
		{ID: "a", Start: 0, End: 10, Value: "A"},
		{ID: "b", Start: 5, End: 15, Value: "B"},
		{ID: "c", Start: 5, End: 8, Value: "C"},
		{ID: "d", Start: 20, End: 30, Value: "D"},
	} {
		if err := ix.Add(e.ID, e.Start, e.End, e.Value); err != nil {
			t.Fatal(err)
		}
	}
	mustOverlap := func(start, end int64) []Entry {
		t.Helper()
		got, err := ix.Overlap(start, end)
		if err != nil {
			t.Fatalf("Overlap(%d, %d): %v", start, end, err)
		}
		return got
	}

	assertIDs(t, mustOverlap(7, 21), []string{"a", "b", "c", "d"})
	assertIDs(t, mustOverlap(10, 20), []string{"b"}) // half-open: a and d only touch
	assertIDs(t, mustOverlap(30, 40), nil)
	assertIDs(t, mustOverlap(-100, 100), []string{"a", "b", "c", "d"})
}

func TestUpdateRelocatesEntry(t *testing.T) {
	ix := New()
	for _, e := range []Entry{
		{ID: "a", Start: 0, End: 10, Value: "A"},
		{ID: "b", Start: 5, End: 15, Value: "B"},
		{ID: "c", Start: 20, End: 30, Value: "C"},
	} {
		if err := ix.Add(e.ID, e.Start, e.End, e.Value); err != nil {
			t.Fatal(err)
		}
	}

	if err := ix.Update("b", 25, 35, "B2"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ix.Len() != 3 {
		t.Fatalf("Len = %d, want 3", ix.Len())
	}
	assertIDs(t, ix.FindPoint(7), []string{"a"})       // b left its old spot
	assertIDs(t, ix.FindPoint(27), []string{"c", "b"}) // b found at new spot

	e, err := ix.Get("b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e != (Entry{ID: "b", Start: 25, End: 35, Value: "B2"}) {
		t.Fatalf("Get returned %+v", e)
	}
	assertIDs(t, ix.Snapshot(), []string{"a", "c", "b"})

	if err := ix.Update("b", 40, 30, "bad"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Update invalid range: got %v, want ErrInvalidRange", err)
	}
	e, _ = ix.Get("b")
	if e.Start != 25 || e.End != 35 {
		t.Fatalf("failed Update mutated entry: %+v", e)
	}
	checkInvariants(t, ix.root)
}

func TestMaxEndAndBalanceAfterRotations(t *testing.T) {
	ix := New()

	// Ascending inserts force left rotations, descending force right ones.
	for i := 0; i < 24; i++ {
		if err := ix.Add(fmt.Sprintf("up%02d", i), int64(i*10), int64(i*10+7), "u"); err != nil {
			t.Fatal(err)
		}
		checkInvariants(t, ix.root)
	}
	for i := 24; i < 48; i++ {
		if err := ix.Add(fmt.Sprintf("dn%02d", i), int64((48-i)*-10), int64((48-i)*-10+3), "d"); err != nil {
			t.Fatal(err)
		}
		checkInvariants(t, ix.root)
	}
	if ix.Len() != 48 {
		t.Fatalf("Len = %d, want 48", ix.Len())
	}

	// Removals (including nodes with two children) keep invariants intact.
	for i := 0; i < 24; i += 2 {
		if err := ix.Remove(fmt.Sprintf("up%02d", i)); err != nil {
			t.Fatal(err)
		}
		checkInvariants(t, ix.root)
	}
	// Updates re-insert at different positions; invariants still hold.
	for i := 1; i < 24; i += 2 {
		if err := ix.Update(fmt.Sprintf("up%02d", i), int64(i*100), int64(i*100+50), "m"); err != nil {
			t.Fatal(err)
		}
		checkInvariants(t, ix.root)
	}

	// Spot-check query correctness against the surviving entries.
	assertIDs(t, ix.FindPoint(115), []string{"up01"})
	assertIDs(t, ix.FindPoint(35), nil) // up03 moved away, up02/up04 removed
	if got := len(ix.Snapshot()); got != ix.Len() {
		t.Fatalf("Snapshot has %d entries, Len = %d", got, ix.Len())
	}
}

func TestSnapshotAndClose(t *testing.T) {
	ix := New()
	for _, e := range []Entry{
		{ID: "a", Start: 0, End: 10, Value: "A"},
		{ID: "b", Start: 5, End: 15, Value: "B"},
		{ID: "c", Start: 20, End: 30, Value: "C"},
	} {
		if err := ix.Add(e.ID, e.Start, e.End, e.Value); err != nil {
			t.Fatal(err)
		}
	}

	snap := ix.Snapshot()
	assertIDs(t, snap, []string{"a", "b", "c"})
	snap[0].Value = "mutated" // snapshot is an independent copy
	if e, _ := ix.Get("a"); e.Value != "A" {
		t.Fatalf("snapshot mutation leaked into index: %+v", e)
	}

	ix.Close()
	ix.Close() // idempotent

	if err := ix.Add("x", 0, 1, "x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Add after Close: got %v, want ErrClosed", err)
	}
	if err := ix.Update("a", 0, 1, "x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Update after Close: got %v, want ErrClosed", err)
	}
	if err := ix.Remove("a"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Remove after Close: got %v, want ErrClosed", err)
	}

	// Reads keep working after Close.
	if e, err := ix.Get("a"); err != nil || e.Value != "A" {
		t.Fatalf("Get after Close: %+v, %v", e, err)
	}
	assertIDs(t, ix.FindPoint(7), []string{"a", "b"})
	if got, err := ix.Overlap(9, 21); err != nil {
		t.Fatalf("Overlap after Close: %v", err)
	} else {
		assertIDs(t, got, []string{"a", "b", "c"})
	}
	assertIDs(t, ix.Snapshot(), []string{"a", "b", "c"})
	if ix.Len() != 3 {
		t.Fatalf("Len after Close = %d, want 3", ix.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	ix := New()
	var wg sync.WaitGroup

	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				id := fmt.Sprintf("w%d-%02d", w, i)
				start := int64(i * 4)
				if err := ix.Add(id, start, start+6, "v"); err != nil {
					t.Errorf("Add: %v", err)
				}
				if i%3 == 0 {
					if err := ix.Update(id, start+100, start+106, "u"); err != nil {
						t.Errorf("Update: %v", err)
					}
				}
				if i%2 == 0 {
					if err := ix.Remove(id); err != nil {
						t.Errorf("Remove: %v", err)
					}
				}
			}
		}(w)
	}
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				ix.FindPoint(int64(i))
				if _, err := ix.Overlap(int64(i), int64(i+10)); err != nil {
					t.Errorf("Overlap: %v", err)
				}
				ix.Snapshot()
				ix.Len()
			}
		}()
	}
	wg.Wait()

	checkInvariants(t, ix.root)
	if got, want := ix.Len(), len(ix.Snapshot()); got != want {
		t.Fatalf("Len = %d, Snapshot has %d", got, want)
	}
}
