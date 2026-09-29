//go:build stephook

package elist_head_test

import (
	"testing"
	"time"

	elist "github.com/kazu/elist_head"
)

// Nodes b, p and q lie in this order between head and tail. Inserting n
// before q stops after it linked n from p, and deleting p stops after it
// marked p. Deleting n must wait for the insert, which has not linked n from
// q yet: it took n out from b instead, and the insert then linked n from q,
// so that q led back to n, out of the list.
func TestMarkForDeleteWaitsForItsInsertAfterAMarkedNode(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	b, p, q, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{b, p, q} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	any := func(*elist.ListHead) bool { return true }

	s := newStepper(t)
	ins := s.stopAt("add.cas2", n)
	doneI, _ := goDo(func() error { return q.TryInsertBefore(n, any) })
	ins.waitReached(t)
	delP := s.stopAt("del.marked", p)
	doneP, _ := goDo(func() error { return p.MarkForDelete() })
	delP.waitReached(t)
	doneN, _ := goDo(func() error { return n.MarkForDelete() })
	select {
	case <-doneN:
	case <-time.After(200 * time.Millisecond):
	}
	ins.Release()
	waitClosed(t, doneI, "insert n")
	waitClosed(t, doneN, "delete n")

	names := map[*elist.ListHead]string{head: "head", b: "b", p: "p", q: "q", n: "n", tail: "tail"}
	assertLinked(t, names, head, tail, "b", "q")
	delP.Release()
	waitClosed(t, doneP, "delete p")
}

// Nodes p, b, a and c lie in this order between head and tail. Inserting n
// before c stops after it linked n from a, and deleting a stops after it
// marked a. Deleting b must wait for the insert, whose node n lies after a:
// it linked p to n and n back to p. Deleting c then stops after it marked c,
// so that the insert fails and puts n back, leaving p leading to n, which
// leads nowhere.
func TestMarkForDeleteWaitsForAnInsertAfterAMarkedNode(t *testing.T) {
	entries := make([]typedEntry, 7)
	head, tail := &entries[0].ListHead, &entries[6].ListHead
	elist.InitAsEmpty(head, tail)
	p, b, a, c, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead, &entries[5].ListHead
	for _, x := range []*elist.ListHead{p, b, a, c} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	any := func(*elist.ListHead) bool { return true }

	s := newStepper(t)
	ins := s.stopAt("add.cas2", n)
	doneI, _ := goDo(func() error { return c.TryInsertBefore(n, any) })
	ins.waitReached(t)
	delA := s.stopAt("del.marked", a)
	doneA, _ := goDo(func() error { return a.MarkForDelete() })
	delA.waitReached(t)
	doneB, _ := goDo(func() error { return b.MarkForDelete() })
	select {
	case <-doneB:
	case <-time.After(200 * time.Millisecond):
	}
	delC := s.stopAt("del.marked", c)
	doneC, _ := goDo(func() error { return c.MarkForDelete() })
	delC.waitReached(t)
	ins.Release()
	waitClosed(t, doneI, "insert n")
	waitClosed(t, doneB, "delete b")

	names := map[*elist.ListHead]string{head: "head", p: "p", b: "b", a: "a", c: "c", n: "n", tail: "tail"}
	assertLinked(t, names, head, tail, "p")
	delA.Release()
	delC.Release()
	waitClosed(t, doneA, "delete a")
	waitClosed(t, doneC, "delete c")
}
