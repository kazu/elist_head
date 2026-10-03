//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// newReplaceList links the nodes named in mid in this order between head and
// tail, and links two more nodes n1 and n2 to each other as the chain that
// ReplaceNext puts in. It returns head, tail, the nodes of mid, n1, n2 and the
// names of all of them.
func newReplaceList(t *testing.T, mid ...string) (head, tail *elist.ListHead, nodes []*elist.ListHead, n1, n2 *elist.ListHead, names map[*elist.ListHead]string) {
	t.Helper()
	entries := make([]typedEntry, len(mid)+4)
	head, tail = &entries[0].ListHead, &entries[len(mid)+1].ListHead
	elist.InitAsEmpty(head, tail)
	names = map[*elist.ListHead]string{head: "head", tail: "tail"}
	for i, name := range mid {
		n := &entries[i+1].ListHead
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
		names[n] = name
	}
	n1, n2 = &entries[len(mid)+2].ListHead, &entries[len(mid)+3].ListHead
	elist.InitAsEmpty(n1, n2)
	names[n1], names[n2] = "n1", "n2"
	return
}

// Nodes p, a and q lie in this order between head and tail. p.ReplaceNext
// replaces a with the chain n1, n2, and stops at "replace.read" after it read
// p.next as a and q.prev as a. a is then deleted to its end, so p.next is q
// and q.prev is p. The replace resumes: its first CAS, p.next from a to n1,
// fails. While it stops at "replace.retry", the list must still hold p and q
// linked both ways; a rollback that stores the a it read into p.next and
// q.prev, although the replace changed neither of them, puts the deleted a
// back until the next try of the replace overwrites p.next and q.prev.
func TestReplaceNextFirstCASFailsKeepsDelete(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, a, q := nodes[0], nodes[1], nodes[2]

	s := newStepper(t)
	read := s.stopAt("replace.read", p)
	done, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	read.waitReached(t)
	if err := a.MarkForDelete(); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	assertLinked(t, names, head, tail, "p", "q")
	rollback := s.stopAt("replace.retry", p)
	read.Release()
	rollback.waitReached(t)

	assertLinked(t, names, head, tail, "p", "q")

	rollback.Release()
	waitClosed(t, done, "replace a")
	if *errR != nil {
		t.Fatalf("replace a: %v", *errR)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q")
}

// Nodes p, a, b and q lie in this order between head and tail. p.ReplaceNext
// replaces a and b with the chain n1, n2, and stops at "replace.read" after
// it read p.next as a and q.prev as b. b is then deleted to its end, so a.next
// is q and q.prev is a; p.next is still a. The replace resumes: its first
// CAS, p.next from a to n1, succeeds, and its second CAS, q.prev from b to n2,
// fails. Its rollback puts p.next back to a, and also stores the b it read
// into q.prev, over the a that the delete wrote. While it stops at
// "replace.rollback", the list must hold p, a and q linked both ways; now the
// backward walk from q goes through the deleted b.
func TestReplaceNextSecondCASFailsKeepsDelete(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "b", "q")
	p, b, q := nodes[0], nodes[2], nodes[3]

	s := newStepper(t)
	read := s.stopAt("replace.read", p)
	done, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	read.waitReached(t)
	if err := b.MarkForDelete(); err != nil {
		t.Fatalf("delete b: %v", err)
	}
	assertLinked(t, names, head, tail, "p", "a", "q")
	rollback := s.stopAt("replace.rollback", p)
	read.Release()
	rollback.waitReached(t)

	assertLinked(t, names, head, tail, "p", "a", "q")

	rollback.Release()
	waitClosed(t, done, "replace a and b")
	if *errR != nil {
		t.Fatalf("replace a and b: %v", *errR)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q")
}
