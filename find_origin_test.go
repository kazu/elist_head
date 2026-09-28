package elist_head_test

import (
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// moveEntries leads the list to dst, the copy of the linked entries of src.
func moveEntries(src, dst []typedEntry) {
	elist.RepaireSliceAfterCopy(
		unsafe.Pointer(&src[0]),
		unsafe.Pointer(&src[len(src)-1]),
		unsafe.Pointer(&dst[0]),
		int(unsafe.Sizeof(src[0])),
		int(unsafe.Offsetof(src[0].ListHead)))
}

// FindOrigin returns the first node of a line of copies for each of them: a
// node, its copy, and the copy of that copy, also after the second move led
// the entry of the first move to the last copy.
func TestFindOriginOverTwoMoves(t *testing.T) {
	ends := make([]typedEntry, 2)
	head, tail := &ends[0].ListHead, &ends[1].ListHead
	elist.InitAsEmpty(head, tail)
	a, b, c := make([]typedEntry, 2), make([]typedEntry, 2), make([]typedEntry, 2)
	for i := range a {
		if _, err := tail.InsertBefore(&a[i].ListHead); err != nil {
			t.Fatal(err)
		}
	}
	moveEntries(a, b)
	moveEntries(b, c)

	for i := range a {
		for name, n := range map[string]*elist.ListHead{"a": &a[i].ListHead, "b": &b[i].ListHead, "c": &c[i].ListHead} {
			if got := elist.FindOrigin(n); got != &a[i].ListHead {
				t.Errorf("FindOrigin(%s[%d]) = %p, want a[%d] %p", name, i, got, i, &a[i].ListHead)
			}
		}
	}
	if got := elist.FindOrigin(head); got != head {
		t.Errorf("FindOrigin(head) = %p, want head %p", got, head)
	}
}
