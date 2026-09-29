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
// fails. The delete linked q back to n2 itself and returned instead, so that
// a was reused while the replace could still put a back into p.next.
func TestDeleteOfAReplacedNodeWaitsForTheReplace(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, a, q := nodes[0], nodes[1], nodes[2]

	s := newStepper(t)
	cas2 := s.stopAt("replace.cas2", p)
	done, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	cas2.waitReached(t)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	select {
	case <-doneD:
		t.Fatalf("the delete of a returned while the replace was between its CASes")
	case <-time.After(200 * time.Millisecond):
	}
	cas2.Release()
	waitClosed(t, done, "replace a")
	waitClosed(t, doneD, "delete a")
	if *errR != nil || *errD != nil {
		t.Fatalf("replace a, delete a = %v, %v, want nil, nil", *errR, *errD)
	}
	a.Init()
	if _, err := tail.InsertBefore(a); err != nil {
		t.Fatalf("insert a again: %v", err)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q", "a")
}
