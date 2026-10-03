//go:build stephook

package elist_head_test

import (
	"strings"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// repairEntries returns the backing array for the RepaireSliceAfterCopy
// tests. The nodes outside the copied slice take the first elements, the
// source slice takes all[10:], and the copy takes all[20:], so a copy lies
// 10 elements after its source. The elements between them have no name: a
// link of a copy that still holds the offset of the source points into them,
// and a walk stops there instead of following it.
func repairEntries() []typedEntry {
	return make([]typedEntry, 32)
}

// repairSlice runs RepaireSliceAfterCopy for src copied to dst.
func repairSlice(src, dst []typedEntry) error {
	return elist.RepaireSliceAfterCopy(
		unsafe.Pointer(&src[0]),
		unsafe.Pointer(&src[len(src)-1]),
		unsafe.Pointer(&dst[0]),
		int(unsafe.Sizeof(src[0])),
		int(unsafe.Offsetof(src[0].ListHead)))
}

// walkNamed walks the list from head to tail, forward or backward, and
// returns the names of the nodes in the order from head to tail. It stops
// with "?" at a link to a node not in names, without following it.
func walkNamed(names map[*elist.ListHead]string, head, tail *elist.ListHead, forward bool) string {
	var got []string
	cur, end := head, tail
	if !forward {
		cur, end = tail, head
	}
	for i := 0; ; i++ {
		if i > len(names) {
			got = append(got, "...")
			break
		}
		if forward {
			cur = cur.DirectNext()
		} else {
			cur = cur.DirectPrev()
		}
		if cur == end {
			break
		}
		name, ok := names[cur]
		if !ok {
			got = append(got, "?")
			break
		}
		got = append(got, name)
	}
	if !forward {
		for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
			got[i], got[j] = got[j], got[i]
		}
	}
	return strings.Join(got, " ")
}

// Nodes a, s0 and b lie in this order between head and tail, and s0 is the
// only element of a slice that is copied to d0. RepaireSliceAfterCopy stops
// after it changed a.next from s0 to d0, before it moves the links of d0.
// A reader walking forward then goes from a to d0, and d0.next still holds
// the offset from s0 to b, so it leads 10 elements past b, which is no node.
// The walk forward must be a, d0, b.
func TestRepairCopyReachableBeforeItsLinks(t *testing.T) {
	all := repairEntries()
	head, a, b, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead
	src := all[10:11]
	elist.InitAsEmpty(head, tail)
	for _, n := range []*elist.ListHead{a, &src[0].ListHead, b} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	dst := append(all[20:20:22], src...)
	names := map[*elist.ListHead]string{
		head: "head", a: "a", b: "b", tail: "tail",
		&src[0].ListHead: "s0", &dst[0].ListHead: "d0",
	}

	s := newStepper(t)
	stop := s.stopAt("repair.prevLinked", &src[0].ListHead)
	done, errRepair := goDo(func() error { return repairSlice(src, dst) })
	stop.waitReached(t)
	if got := walkNamed(names, head, tail, true); got != "a d0 b" {
		t.Errorf("forward while d0 is reachable from a: %q, want %q", got, "a d0 b")
	}
	stop.Release()
	waitClosed(t, done, "repair")
	if *errRepair != nil {
		t.Fatalf("repair: %v", *errRepair)
	}
	for _, forward := range []bool{true, false} {
		if got := walkNamed(names, head, tail, forward); got != "a d0 b" {
			t.Errorf("after repair, forward=%v: %q, want %q", forward, got, "a d0 b")
		}
	}
}
