//go:build stephook && race

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes a, s0 and b lie in this order between head and tail, and s0 is the
// only element of a slice that is copied to d0. RepaireSliceAfterCopy stops
// after it changed a.next from s0 to d0. A reader starts, loads a.next and
// reads the links of d0 with atomic loads. The repair is released without
// waiting for the reader, and writes d0.prev and d0.next with plain stores.
// The reader synchronizes only with the CAS on a.next, which comes before
// those stores, and the release does not wait for the reader, so either
// order of the reads and the stores is a data race.
func TestRepairCopyLinksWrittenAfterPublishRace(t *testing.T) {
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

	s := newStepper(t)
	stop := s.stopAt("repair.prevLinked", &src[0].ListHead)
	done, errRepair := goDo(func() error { return repairSlice(src, dst) })
	stop.waitReached(t)
	read := make(chan struct{})
	go func() {
		defer close(read)
		n := a.DirectNext()
		_ = n.DirectNext()
		_ = n.IsMarked()
	}()
	stop.Release()
	waitClosed(t, done, "repair")
	waitClosed(t, read, "read")
	if *errRepair != nil {
		t.Fatalf("repair: %v", *errRepair)
	}
}
