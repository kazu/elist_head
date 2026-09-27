//go:build stephook

package elist_head_test

import "testing"

// Nodes p, a and q lie in this order between head and tail. p.ReplaceNext
// replaces a with the chain n1, n2. It reads p.next as a and q.prev as a, and
// its first CAS changes p.next from a to n1; it stops at "replace.cas2",
// before its second CAS. a is then deleted to its end: p.next is no longer a,
// so the delete links q back to n2, the node that links to q walking forward
// from p. a is reused and linked again between q and tail. The replace
// resumes: its second CAS, q.prev from a to n2, fails. The list must hold p,
// n1, n2, q and a linked both ways, also while the replace stops at
// "replace.rollback"; a rollback that stores the a it read into p.next makes
// the forward walk pass over q to the reused a.
func TestReplaceNextRollbackRestoresReusedNode(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, a, q := nodes[0], nodes[1], nodes[2]

	s := newStepper(t)
	cas2 := s.stopAt("replace.cas2", p)
	done, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	cas2.waitReached(t)
	if err := a.MarkForDelete(); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	a.Init()
	if _, err := tail.InsertBefore(a); err != nil {
		t.Fatalf("insert a again: %v", err)
	}
	rollback := s.stopAt("replace.rollback", p)
	cas2.Release()
	select {
	case <-rollback.reached:
		assertLinked(t, names, head, tail, "p", "n1", "n2", "q", "a")
		rollback.Release()
	case <-done:
	}
	waitClosed(t, done, "replace a")
	if *errR != nil {
		t.Fatalf("replace a: %v", *errR)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q", "a")
}

// The same as TestReplaceNextRollbackRestoresReusedNode, and x is inserted
// before q after a is reused, so q.prev is x and x.prev is n2 when the second
// CAS of the replace fails. The replace must end with p, n1, n2, x, q and a
// linked both ways.
func TestReplaceNextSecondCASFailsAfterInsertBeforeNext(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q", "x")
	p, a, q, x := nodes[0], nodes[1], nodes[2], nodes[3]
	if err := x.MarkForDelete(); err != nil {
		t.Fatalf("take x out: %v", err)
	}
	x.Init()

	s := newStepper(t)
	cas2 := s.stopAt("replace.cas2", p)
	done, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	cas2.waitReached(t)
	if err := a.MarkForDelete(); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	a.Init()
	if _, err := tail.InsertBefore(a); err != nil {
		t.Fatalf("insert a again: %v", err)
	}
	if _, err := q.InsertBefore(x); err != nil {
		t.Fatalf("insert x: %v", err)
	}
	cas2.Release()
	waitClosed(t, done, "replace a")
	if *errR != nil {
		t.Fatalf("replace a: %v", *errR)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "x", "q", "a")
}
