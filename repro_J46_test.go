//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes a, s0, s1, s2 and b lie in this order between head and tail, and
// s0, s1 and s2 are a slice that is copied to d0, d1 and d2. The copier
// copies the slice first and then calls RepaireSliceAfterCopy, as
// samepleItemPool._expand does. Between the two, another caller deletes s1,
// which changes only links inside the slice: s0.next and s2.prev now skip s1,
// while d0.next and d2.prev in the copy still lead to d1.
// RepaireSliceAfterCopy only moves the links that leave the slice (a.next and
// b.prev) to the copy, and it does not look at the links inside it, so the
// list it publishes goes through d1, a copy of the deleted s1. The walks must
// be a, d0, d2, b.
func TestRepairPublishesInnerLinksChangedAfterCopy(t *testing.T) {
	all := repairEntries()
	head, a, b, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead
	src := all[10:13]
	elist.InitAsEmpty(head, tail)
	for _, n := range []*elist.ListHead{a, &src[0].ListHead, &src[1].ListHead, &src[2].ListHead, b} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	names := map[*elist.ListHead]string{
		head: "head", a: "a", b: "b", tail: "tail",
		&src[0].ListHead: "s0", &src[1].ListHead: "s1", &src[2].ListHead: "s2",
	}

	// The copier copies the slice.
	dst := append(all[20:20:23], src...)
	names[&dst[0].ListHead] = "d0"
	names[&dst[1].ListHead] = "d1"
	names[&dst[2].ListHead] = "d2"

	// Another caller deletes s1 before the copier repairs the copy.
	if _, err := src[1].ListHead.Delete(); err != nil {
		t.Fatalf("delete s1: %v", err)
	}
	for _, forward := range []bool{true, false} {
		if got := walkNamed(names, head, tail, forward); got != "a s0 s2 b" {
			t.Fatalf("after delete, forward=%v: %q, want %q", forward, got, "a s0 s2 b")
		}
	}

	// The copier repairs the copy.
	if err := repairSlice(src, dst); err != nil {
		t.Fatalf("repair: %v", err)
	}
	for _, forward := range []bool{true, false} {
		if got := walkNamed(names, head, tail, forward); got != "a d0 d2 b" {
			t.Errorf("after repair, forward=%v: %q, want %q", forward, got, "a d0 d2 b")
		}
	}
}
