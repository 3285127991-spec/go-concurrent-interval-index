// Command intervalcli demonstrates the intervalindex library: adding
// entries, point and overlap queries, update, remove, and reading after
// Close.
package main

import (
	"fmt"
	"log"

	"intervalindex"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func show(label string, entries []intervalindex.Entry) {
	fmt.Println(label)
	if len(entries) == 0 {
		fmt.Println("  (none)")
	}
	for _, e := range entries {
		fmt.Printf("  id=%-3q [%2d, %2d) value=%q\n", e.ID, e.Start, e.End, e.Value)
	}
}

func main() {
	ix := intervalindex.New()

	fmt.Println("== Add a:[0,10) b:[5,15) c:[20,30) d:[5,8) ==")
	must(ix.Add("a", 0, 10, "alpha"))
	must(ix.Add("b", 5, 15, "bravo"))
	must(ix.Add("c", 20, 30, "charlie"))
	must(ix.Add("d", 5, 8, "delta"))
	fmt.Println("Len:", ix.Len())

	show("\n== FindPoint(7) ==", ix.FindPoint(7))

	ov, err := ix.Overlap(12, 25)
	must(err)
	show("\n== Overlap(12, 25) ==", ov)

	fmt.Println("\n== Update b -> [18, 28) ==")
	must(ix.Update("b", 18, 28, "bravo-moved"))
	show("FindPoint(7) after update:", ix.FindPoint(7))
	show("FindPoint(25) after update:", ix.FindPoint(25))

	fmt.Println("\n== Remove a ==")
	must(ix.Remove("a"))
	fmt.Println("Len:", ix.Len())

	fmt.Println("\n== Close (twice, idempotent) ==")
	ix.Close()
	ix.Close()
	fmt.Println("Add after Close:   ", ix.Add("x", 0, 1, "nope"))
	fmt.Println("Update after Close:  ", ix.Update("c", 0, 1, "nope"))
	fmt.Println("Remove after Close:", ix.Remove("c"))

	fmt.Println("\n== Reads still work after Close ==")
	show("FindPoint(22):", ix.FindPoint(22))
	show("Snapshot:", ix.Snapshot())
	fmt.Println("Len:", ix.Len())
}
