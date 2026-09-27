//go:build stephook

package elist_head_test

import (
	"fmt"
	"testing"

	elist "github.com/kazu/elist_head"
)

// The same interleaving as TestInsertBeforeNodeDeletedBetweenCASes, with
// TryInsertBefore, which the map uses. Inserting n before a stops at
// "add.cas2" after its first CAS: p.next is n and a.prev is still p. Deleting
// a marks a in another goroutine, and the insert resumes: its second CAS
// fails on the mark of a.prev. When both end, the list must hold p and y
// linked both ways, a must be safe to reuse, TryInsertBefore must return an
// error, and n must be alone.
func TestTryInsertBeforeNodeDeletedBetweenCASes(t *testing.T) {
	head, tail, p, a, y, n, names := newInsertDeleteList(t)
	_, _ = p, y

	s := newStepper(t)
	ins := s.stopAt("add.cas2", n)
	doneI, errI := goDo(func() error {
		return a.TryInsertBefore(n, func(*elist.ListHead) bool { return true })
	})
	ins.waitReached(t)
	marked := s.stopAt("del.marked", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	marked.waitReached(t)
	marked.Release()
	ins.Release()
	waitClosed(t, doneI, "insert n")
	waitClosed(t, doneD, "delete a")
	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}

	assertLinked(t, names, head, tail, "p", "y")
	if safe, _ := a.IsSafety(); !safe {
		t.Error("a is not safe to reuse after its delete")
	}
	if *errI == nil {
		t.Error("TryInsertBefore(n) returned no error, but n is not linked")
	}
	if err := alone(names, n); err != nil {
		t.Error(err)
	}
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a stops at "add.cas2" after its first CAS: p.next is n and a.prev
// is still p. a is then deleted with Delete: MarkForDelete skips relinking
// p.next, which is n, and IsSafety sees p.next is n and y.prev is p, so a is
// Inited and links only to itself. The insert resumes: its second CAS fails,
// its rollback puts p.next back to a, and its retry reads a itself as the
// node before a, so listAddWitCas(n, a, a) succeeds. InsertBefore returns
// nil, p.next is the Inited a, and a and n make a ring. The list must hold p
// and y linked both ways, the insert must return an error, and n must stay
// alone.
func TestInsertBeforeNodeInitedBetweenCASes(t *testing.T) {
	head, tail, p, a, y, n, names := newInsertDeleteList(t)
	_, _ = p, y

	s := newStepper(t)
	ins := s.stopAt("add.cas2", n)
	doneI, errI := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)
	marked := s.stopAt("del.marked", a)
	doneD, errD := goDo(func() error { _, err := a.Delete(); return err })
	marked.waitReached(t)
	marked.Release()
	ins.Release()
	waitClosed(t, doneI, "insert n")
	waitClosed(t, doneD, "delete a")
	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}

	assertLinked(t, names, head, tail, "p", "y")
	if *errI == nil {
		t.Error("InsertBefore(n) returned no error, but n is not linked")
	}
	if err := alone(names, n); err != nil {
		t.Error(err)
	}
}

// wantInsertedOrAlone returns an error unless the list holds p, n and y
// when the insert of n returned nil, or holds p and y and n is alone when
// it returned an error.
func wantInsertedOrAlone(byNode map[*elist.ListHead]string, head, tail, n *elist.ListHead, errInsert error) error {
	if errInsert == nil {
		return wantLinked(byNode, head, tail, "p", "n", "y")
	}
	if err := wantLinked(byNode, head, tail, "p", "y"); err != nil {
		return err
	}
	return alone(byNode, n)
}

// n is inserted before a while a is deleted with MarkForDelete.
func TestExploreInsertBeforeDeletedNode(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "p", "a", "y", "n")
		p, a, y, n := nodes["p"], nodes["a"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, a, y)
		return []op{insertOp("insert n", a, n), deleteOp("delete a", a)}, func(errs []error) error {
			if err := checkDeleted(byNode, a, errs[1]); err != nil {
				return err
			}
			return wantInsertedOrAlone(byNode, head, tail, n, errs[0])
		}
	})
}

// n is inserted before a while a is deleted with Delete, which Inits a after
// unlinking it.
func TestExploreInsertBeforeDeletedAndInitedNode(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "p", "a", "y", "n")
		p, a, y, n := nodes["p"], nodes["a"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, a, y)
		del := op{name: "delete a", nodes: []*elist.ListHead{a}, do: func() error { _, err := a.Delete(); return err }}
		return []op{insertOp("insert n", a, n), del}, func(errs []error) error {
			if errs[1] != nil {
				return fmt.Errorf("delete a: %v", errs[1])
			}
			if err := wantUnlinked(byNode, a); err != nil {
				return err
			}
			if err := wantInsertedOrAlone(byNode, head, tail, n, errs[0]); err != nil {
				return err
			}
			return alone(byNode, a)
		}
	})
}
