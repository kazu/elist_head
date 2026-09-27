//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes a, s0 and s1 lie in this order between head and tail, and s0 and s1
// are a slice that is copied to d0 and d1. RepaireSliceAfterCopy stops after
// it changed a.next from s0 to d0. X is then inserted before d0, so a.next
// and d0.prev lead to X, and N is inserted before tail, so the repair can no
// longer move tail.prev from s1 to d1. The repair then has to lead the list
// back to s0 while a.next no longer points to d0. Whether the repair fails or
// not, the list must stay linked the same way in both directions, through
// either the source or the copy.
func TestRepairPuttingBackALinkChangedMeanwhileLeavesListLinked(t *testing.T) {
	all := repairEntries()
	head, a, x, n, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead, &all[4].ListHead
	src := all[10:12]
	elist.InitAsEmpty(head, tail)
	for _, e := range []*elist.ListHead{a, &src[0].ListHead, &src[1].ListHead} {
		if _, err := tail.InsertBefore(e); err != nil {
			t.Fatal(err)
		}
	}
	dst := append(all[20:20:24], src...)

	s := newStepper(t)
	stop := s.stopAt("repair.prevLinked", &src[0].ListHead)
	done, errRepair := goDo(func() error { return repairSlice(src, dst) })
	stop.waitReached(t)
	if _, err := dst[0].ListHead.InsertBefore(x); err != nil {
		t.Fatalf("insert X: %v", err)
	}
	if _, err := tail.InsertBefore(n); err != nil {
		t.Fatalf("insert N: %v", err)
	}
	stop.Release()
	waitClosed(t, done, "repair")

	names := map[*elist.ListHead]string{
		head: "head", a: "a", x: "X", n: "N", tail: "tail",
		&src[0].ListHead: "s0", &src[1].ListHead: "s1",
		&dst[0].ListHead: "d0", &dst[1].ListHead: "d1",
	}
	fwd := walkNamed(names, head, tail, true)
	bwd := walkNamed(names, head, tail, false)
	if fwd != bwd || (fwd != "a X s0 s1 N" && fwd != "a X d0 d1 N") {
		t.Errorf("repair returned %v; forward %q, backward %q, want both %q or both %q",
			*errRepair, fwd, bwd, "a X s0 s1 N", "a X d0 d1 N")
	}
}

// Nodes a, s0, b and s1 lie in this order between head and tail, and s0 and
// s1 are a slice that is copied to d0 and d1. RepaireSliceAfterCopy moves
// a.next, b.prev and b.next to the copy and stops at repair.prevLinked. N is
// then inserted before tail, so the repair can no longer move tail.prev and
// leads the list back to the source. It moves b.prev back to s0 and stops at
// repair.putBack, before it goes on to a.next. Z is then inserted before b: it
// reads s0 as the previous node of b and links itself between s0 and b. When
// the repair resumes, s0.next must keep leading to Z.
func TestRepairPuttingBackKeepsAnInsertNextToTheSource(t *testing.T) {
	all := repairEntries()
	head, a, b, n, z, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead, &all[4].ListHead, &all[5].ListHead
	src := all[10:12]
	elist.InitAsEmpty(head, tail)
	for _, e := range []*elist.ListHead{a, &src[0].ListHead, b, &src[1].ListHead} {
		if _, err := tail.InsertBefore(e); err != nil {
			t.Fatal(err)
		}
	}
	dst := append(all[20:20:24], src...)

	s := newStepper(t)
	linked := s.stopAt("repair.prevLinked", &src[1].ListHead)
	putBack := s.stopAt("repair.putBack", &src[0].ListHead)
	done, errRepair := goDo(func() error { return repairSlice(src, dst) })
	linked.waitReached(t)
	if _, err := tail.InsertBefore(n); err != nil {
		t.Fatalf("insert N: %v", err)
	}
	linked.Release()
	putBack.waitReached(t)
	if _, err := b.InsertBefore(z); err != nil {
		t.Fatalf("insert Z: %v", err)
	}
	putBack.Release()
	waitClosed(t, done, "repair")

	names := map[*elist.ListHead]string{
		head: "head", a: "a", b: "b", n: "N", z: "Z", tail: "tail",
		&src[0].ListHead: "s0", &src[1].ListHead: "s1",
		&dst[0].ListHead: "d0", &dst[1].ListHead: "d1",
	}
	fwd := walkNamed(names, head, tail, true)
	bwd := walkNamed(names, head, tail, false)
	if fwd != bwd || fwd != "a s0 Z b s1 N" {
		t.Errorf("repair returned %v; forward %q, backward %q, want both %q",
			*errRepair, fwd, bwd, "a s0 Z b s1 N")
	}
}

// Nodes a, s0, b and s1 lie in this order between head and tail, and s0 and
// s1 are a slice that is copied to d0 and d1. An insert of Z before b reads
// s0 as the previous node of b and stops at add.cas1. RepaireSliceAfterCopy
// moves a.next, b.prev and b.next to the copy and stops at
// repair.prevLinked; N is then inserted before tail, so the repair fails. The
// insert of Z changes s0.next to Z and stops at add.cas2, waiting for b.prev
// to be s0 again. The repair then leads the list back to the source: it must
// leave s0.next leading to Z, so that the insert of Z ends with Z linked in
// both directions.
func TestRepairPuttingBackKeepsAnInsertWaitingForTheSource(t *testing.T) {
	all := repairEntries()
	head, a, b, n, z, tail := &all[0].ListHead, &all[1].ListHead, &all[2].ListHead, &all[3].ListHead, &all[4].ListHead, &all[5].ListHead
	src := all[10:12]
	elist.InitAsEmpty(head, tail)
	for _, e := range []*elist.ListHead{a, &src[0].ListHead, b, &src[1].ListHead} {
		if _, err := tail.InsertBefore(e); err != nil {
			t.Fatal(err)
		}
	}
	dst := append(all[20:20:24], src...)

	s := newStepper(t)
	cas1 := s.stopAt("add.cas1", z)
	cas2 := s.stopAt("add.cas2", z)
	doneZ, errZ := goDo(func() error { _, err := b.InsertBefore(z); return err })
	cas1.waitReached(t)

	linked := s.stopAt("repair.prevLinked", &src[1].ListHead)
	done, errRepair := goDo(func() error { return repairSlice(src, dst) })
	linked.waitReached(t)
	if _, err := tail.InsertBefore(n); err != nil {
		t.Fatalf("insert N: %v", err)
	}
	cas1.Release()
	cas2.waitReached(t)
	linked.Release()
	waitClosed(t, done, "repair")
	cas2.Release()
	waitClosed(t, doneZ, "insert Z")
	if *errZ != nil {
		t.Fatalf("insert Z: %v", *errZ)
	}

	names := map[*elist.ListHead]string{
		head: "head", a: "a", b: "b", n: "N", z: "Z", tail: "tail",
		&src[0].ListHead: "s0", &src[1].ListHead: "s1",
		&dst[0].ListHead: "d0", &dst[1].ListHead: "d1",
	}
	fwd := walkNamed(names, head, tail, true)
	bwd := walkNamed(names, head, tail, false)
	if fwd != bwd || fwd != "a s0 Z b s1 N" {
		t.Errorf("repair returned %v; forward %q, backward %q, want both %q",
			*errRepair, fwd, bwd, "a s0 Z b s1 N")
	}
}
