//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes p, a and q lie in this order between head and tail. p is deleted and
// stops at "del.nextMarked": it read p.prev as head and p.next as a, and has
// marked p.next. p.ReplaceNext then replaces a with the chain n1, n2 to its
// end: it reads p.next with the mark, and its first CAS takes that marked
// value as the one to expect, so it succeeds and stores n1 without the mark.
// Right after the replace, p.next must still carry the mark of the delete;
// now neither link of p is marked. The delete resumes and unlinks p with the
// a it read: head.next becomes a and a.prev head, while n1.prev also becomes
// head and q.prev stays n2. The list must hold the same nodes walking forward
// and backward; now the forward walk holds a and q, and the backward walk n1,
// n2 and q.
func TestReplaceNextFirstCASClearsDeleteMark(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, q := nodes[0], nodes[2]

	s := newStepper(t)
	marked := s.stopAt("del.nextMarked", p)
	doneD, errD := goDo(func() error { return p.MarkForDelete() })
	marked.waitReached(t)

	if err := p.ReplaceNext(n1, n2, q); err != nil {
		t.Logf("replace a: %v", err)
	}
	if _, next := elist.LinkMarks(p); !next {
		t.Errorf("p.next lost the mark of the delete of p after the replace")
	}

	marked.Release()
	waitClosed(t, doneD, "delete p")
	if *errD != nil {
		t.Fatalf("delete p: %v", *errD)
	}
	if _, err := linked(names, head, tail); err != nil {
		t.Errorf("after the delete of p: %v", err)
	}
}
