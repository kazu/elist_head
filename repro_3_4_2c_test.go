//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes a, s0, b and s1 lie in this order between head and tail, and s0 and
// s1 are a slice that is copied to d0 and d1. After the copy and before
// RepaireSliceAfterCopy, N is inserted before s1, as an inserter that does
// not wait for the copy does. The repair moves a.next and b.prev to d0, then
// moves N.next to d1, finds that d1.prev still points to b, and returns an
// error without putting the moved links back. The list is left linked
// forward through d0 and d1, whose next still holds the offset from s1 to
// tail, and backward through s1 and s0. Whether the repair fails or not, the
// list must stay linked the same way in both directions, through either the
// source or the copy.
func TestRepairErrorLeavesListLinked(t *testing.T) {
	all := repairEntries()
	head, a, b, n, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead, &all[4].ListHead
	src := all[10:12]
	elist.InitAsEmpty(head, tail)
	for _, x := range []*elist.ListHead{a, &src[0].ListHead, b, &src[1].ListHead} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	dst := append(all[20:20:24], src...)
	if _, err := src[1].InsertBefore(n); err != nil {
		t.Fatal(err)
	}
	err := repairSlice(src, dst)

	names := map[*elist.ListHead]string{
		head: "head", a: "a", b: "b", n: "N", tail: "tail",
		&src[0].ListHead: "s0", &src[1].ListHead: "s1",
		&dst[0].ListHead: "d0", &dst[1].ListHead: "d1",
	}
	fwd := walkNamed(names, head, tail, true)
	bwd := walkNamed(names, head, tail, false)
	if fwd != bwd || (fwd != "a s0 b N s1" && fwd != "a d0 b N d1") {
		t.Errorf("repair returned %v; forward %q, backward %q, want both %q or both %q",
			err, fwd, bwd, "a s0 b N s1", "a d0 b N d1")
	}
}
