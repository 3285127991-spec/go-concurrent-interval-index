// Command intervalindex demonstrates the intervalindex library:
// add, point query, overlap query, update, remove, and reads after Close.
package main

import (
	"fmt"
	"log"

	"intervalindex"
)

func printEntries(label string, entries []intervalindex.Entry) {
	fmt.Printf("%s (%d):\n", label, len(entries))
	for _, e := range entries {
		fmt.Printf("  %-4s [%d, %d)  %q\n", e.ID, e.Start, e.End, e.Value)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	ix := intervalindex.New()

	fmt.Println("== Add ==")
	must(ix.Add("a", 0, 10, "alpha"))
	must(ix.Add("b", 5, 15, "bravo"))
	must(ix.Add("c", 20, 30, "charlie"))
	must(ix.Add("d", 25, 40, "delta"))
	fmt.Println("added a[0,10) b[5,15) c[20,30) d[25,40); Len =", ix.Len())

	fmt.Println("\n== FindPoint(7) ==")
	printEntries("entries containing 7", ix.FindPoint(7))

	fmt.Println("\n== Overlap(12, 26) ==")
	hits, err := ix.Overlap(12, 26)
	must(err)
	printEntries("entries overlapping [12, 26)", hits)

	fmt.Println("\n== Update(b -> [8, 12)) ==")
	must(ix.Update("b", 8, 12, "bravo-2"))
	printEntries("entries containing 7", ix.FindPoint(7))

	fmt.Println("\n== Remove(c) ==")
	must(ix.Remove("c"))
	fmt.Println("Len =", ix.Len())

	fmt.Println("\n== Close ==")
	ix.Close()
	ix.Close() // idempotent
	if err := ix.Add("e", 50, 60, "echo"); err != nil {
		fmt.Println("Add after Close rejected:", err)
	}
	fmt.Println("reads still work after Close:")
	printEntries("entries containing 25", ix.FindPoint(25))
	printEntries("snapshot", ix.Snapshot())
	fmt.Println("Len =", ix.Len())
}
