//go:build stephook

package elist_head_test

import (
	"testing"
	"time"
)

// Nodes p, a and q lie in this order between head and tail. p.ReplaceNext
// replaces a with the chain n1, n2, and stops before its second CAS: p.next
// is n1, and q.prev is still a. A delete of a must wait for the replace,
// which links q back to n2 or puts a back into p.next when its second CAS
// fails.
func TestDeleteOfAReplacedNodeWaitsForTheReplace(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, a, q := nodes[0], nodes[1], nodes[2]

	s := newStepper(t)
	cas2 := s.stopAt("replace.cas2", p)
	done, _ := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	cas2.waitReached(t)
	doneD, _ := goDo(func() error { return a.MarkForDelete() })
	select {
	case <-doneD:
		t.Fatalf("the delete of a returned while the replace was between its CASes")
	case <-time.After(200 * time.Millisecond):
	}
	cas2.Release()
	waitClosed(t, done, "replace a")
	waitClosed(t, doneD, "delete a")
	a.Init()
	if _, err := tail.InsertBefore(a); err != nil {
		t.Fatalf("insert a again: %v", err)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q", "a")
}

// Nodes p, a and q lie in this order between head and tail. A delete of a
// leads p to q and stops. p.ReplaceNext then replaces the nodes between p and
// q with the chain n1, n2, and stops before its second CAS. The delete goes
// on, and links q back to n2, the node that leads to q from p. x is inserted
// before q, between n2 and q. The replace fails its second CAS, as q.prev is
// x, and finds that q leads back to n2 through x; it must end with p, n1, n2,
// x and q linked both ways.
func TestReplaceNextKeepsAnInsertAfterADeleteLinkedNextBack(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q", "x")
	p, a, q, x := nodes[0], nodes[1], nodes[2], nodes[3]
	if err := x.MarkForDelete(); err != nil {
		t.Fatalf("take x out: %v", err)
	}
	x.Init()

	s := newStepper(t)
	relinked := s.stopAt("del.prevRelinked", a)
	doneD, _ := goDo(func() error { return a.MarkForDelete() })
	relinked.waitReached(t)
	cas2 := s.stopAt("replace.cas2", p)
	done, _ := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	cas2.waitReached(t)
	relinked.Release()
	waitClosed(t, doneD, "delete a")
	if _, err := q.InsertBefore(x); err != nil {
		t.Fatalf("insert x: %v", err)
	}
	cas2.Release()
	waitClosed(t, done, "replace a")
	assertLinked(t, names, head, tail, "p", "n1", "n2", "x", "q")
}
