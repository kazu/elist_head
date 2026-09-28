//go:build stephook

package elist_head_test

import (
	"runtime"
	"testing"

	elist "github.com/kazu/elist_head"
)

// FindOrigin reads the table of moves once. G1 finds the origin of the node
// of a and stops after it read the table; a moves to b; G1 goes on, and must
// return what FindOrigin of the copy in b returns. It read the table again
// to go on to the copy, and returned the copy, as the first read had no entry
// that leads back from it, while FindOrigin of the copy returned the node of
// a.
func TestFindOriginReadsTheTableOnce(t *testing.T) {
	ends := make([]typedEntry, 2)
	head, tail := &ends[0].ListHead, &ends[1].ListHead
	elist.InitAsEmpty(head, tail)
	x, a := make([]typedEntry, 1), make([]typedEntry, 1)
	for _, n := range []*elist.ListHead{&x[0].ListHead, &a[0].ListHead} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	// a move of x first, so that G1 reads a table with entries
	moveEntries(x, make([]typedEntry, 1))

	s := newStepper(t)
	st := s.stopAt("origin.loaded", &a[0].ListHead)
	var got *elist.ListHead
	done, _ := goDo(func() error {
		got = elist.FindOrigin(&a[0].ListHead)
		return nil
	})
	st.waitReached(t)
	b := make([]typedEntry, 1)
	moveEntries(a, b)
	st.Release()
	waitClosed(t, done, "FindOrigin(a)")

	if want := elist.FindOrigin(&b[0].ListHead); got != want {
		t.Errorf("FindOrigin(a) = %p and FindOrigin(b) = %p, want one node", got, want)
	}
	runtime.KeepAlive(a)
}
