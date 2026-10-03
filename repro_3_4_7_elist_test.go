//go:build stephook

package elist_head_test

import (
	"fmt"
	"testing"

	elist "github.com/kazu/elist_head"
	list_head "github.com/kazu/lista_encabezado"
)

// Next and Prev waiting for no mark take the neighbour of a node with one
// atomic load of its link, and directNext and directPrev clear the mark bit of
// that link before they add it to the address of the node. So a reader that
// stands on a node being deleted gets the old neighbour, a real node, or nil
// when the neighbour stays marked; it never gets the marked link itself as a
// node, as lista_encabezado does.
//
// The reader runs as a third goroutine of explore without step points of its
// own, so each schedule puts its reads, all of them at once, between two steps
// of the delete and the insert, and explore puts them between every two steps.

// waitNoMarkReadOp reads Next and Prev of every node of byNode in the
// wait-for-no-mark mode, and returns an error when one of them is neither nil
// nor a node of byNode, or is a marked node. It counts in midDelete the
// schedules where it read while l was marked and p still linked to l, that is,
// while the delete of l was between its first mark and its relink of p.next.
func waitNoMarkReadOp(byNode map[*elist.ListHead]string, p, l *elist.ListHead, midDelete *int) op {
	return op{name: "read", do: func() error {
		if l.IsMarked() && p.DirectNext() == l {
			*midDelete++
		}
		prevs := elist.SharedTrav(list_head.WaitNoM())
		defer elist.SharedTrav(prevs...)
		for m, name := range byNode {
			for dir, got := range map[string]*elist.ListHead{"Next": m.Next(), "Prev": m.Prev()} {
				if got == nil {
					continue
				}
				if _, ok := byNode[got]; !ok {
					return fmt.Errorf("%s.%s() = %p, not a node of the list", name, dir, got)
				}
				if got.IsMarked() {
					return fmt.Errorf("%s.%s() = marked %s", name, dir, byNode[got])
				}
			}
		}
		return nil
	}}
}

// n is inserted before y, after l, while l is deleted, and a reader reads the
// links of every node in between any two steps of them. The insert changes
// l.next while the delete marks it and relinks p.next and y.prev.
func TestExploreWaitNoMarkOnNodeBeingDeletedWithInsertAfter(t *testing.T) {
	midDelete := 0
	explore(t, func() ([]op, func([]error) error) {
		_, tail, nodes, byNode := newNamedList(t, "p", "l", "y", "n")
		p, l, y, n := nodes["p"], nodes["l"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, l, y)
		return []op{insertOp("insert n", y, n), deleteOp("delete l", l), waitNoMarkReadOp(byNode, p, l, &midDelete)},
			func(errs []error) error { return errs[2] }
	})
	if midDelete == 0 {
		t.Errorf("no schedule read while l was marked and still linked from p")
	}
	t.Logf("%d schedules read in the middle of the delete", midDelete)
}

// tryInsertOp links n before at with TryInsertBefore, which makes one try of
// the CASes InsertBefore repeats until one succeeds.
func tryInsertOp(name string, at, n *elist.ListHead) op {
	return op{name: name, nodes: []*elist.ListHead{n}, do: func() error {
		return at.TryInsertBefore(n, func(*elist.ListHead) bool { return true })
	}}
}

// n is inserted before l while l is deleted, and a reader reads the links of
// every node in between any two steps of them. The insert changes l.prev
// while the delete marks it and relinks p.next and y.prev. The insert makes
// one try: InsertBefore retries while l.prev is marked, and the schedules of
// its retries against the delete do not end in minutes; each retry changes
// l.prev with the same CAS as the one try.
func TestExploreWaitNoMarkOnNodeBeingDeletedWithInsertBefore(t *testing.T) {
	midDelete := 0
	explore(t, func() ([]op, func([]error) error) {
		_, tail, nodes, byNode := newNamedList(t, "p", "l", "y", "n")
		p, l, y, n := nodes["p"], nodes["l"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, l, y)
		return []op{tryInsertOp("insert n", l, n), deleteOp("delete l", l), waitNoMarkReadOp(byNode, p, l, &midDelete)},
			func(errs []error) error { return errs[2] }
	})
	if midDelete == 0 {
		t.Errorf("no schedule read while l was marked and still linked from p")
	}
	t.Logf("%d schedules read in the middle of the delete", midDelete)
}
