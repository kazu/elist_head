//go:build stephook && race

package elist_head_test

import (
	"runtime"
	"testing"

	elist "github.com/kazu/elist_head"
)

// runOneAfterOther runs first to its end and then second, each in its own
// goroutine, with nothing that orders second after first for the race
// detector. The test goroutine waits for first to end by watching the number
// of goroutines, which the race detector does not take as synchronization;
// waiting on a channel, a WaitGroup or an atomic would order second after
// first and hide the race. So the only happens-before edges between the two
// are the ones the list code makes itself.
func runOneAfterOther(first, second func()) {
	before := runtime.NumGoroutine()
	doneFirst, doneSecond := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(doneFirst)
		first()
	}()
	for runtime.NumGoroutine() > before {
		runtime.Gosched()
	}
	go func() {
		defer close(doneSecond)
		second()
	}()
	<-doneFirst
	<-doneSecond
}

// Nodes p, a and y lie in this order between head and tail, and n is a single
// node. Deleting a runs to its end first: it moves p.next from a to y and
// y.prev from a to p with CAS, and after each CAS it reads the same link
// plainly again, in the loop over its two neighbors and in the check that
// nothing links to a any more. Inserting n before y runs next: it reads p as
// the node before y, and its CASes move p.next from y to n and y.prev from p
// to n. Each CAS of the insert takes in the delete's CAS on the same link, but
// nothing orders it after the plain reads that follow that CAS, so the race
// detector reports the plain reads of p.next and y.prev in MarkForDelete
// against the CASes of the insert. The delete must read the links
// atomically, and the list must then hold p, n and y.
func TestMarkForDeleteReadsLinksAtomically(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	names := map[*elist.ListHead]string{head: "head", p: "p", a: "a", y: "y", n: "n", tail: "tail"}

	var errDelete, errInsert error
	runOneAfterOther(
		func() { errDelete = a.MarkForDelete() },
		func() { _, errInsert = y.InsertBefore(n) },
	)
	if errDelete != nil {
		t.Fatalf("delete a: %v", errDelete)
	}
	if errInsert != nil {
		t.Fatalf("insert n before y: %v", errInsert)
	}

	assertLinked(t, names, head, tail, "p", "n", "y")
}
