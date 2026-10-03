//go:build stephook

package elist_head_test

import (
	"fmt"
	"testing"

	elist "github.com/kazu/elist_head"
)

// newInsertDeleteList links p, a and y in this order between head and tail,
// and returns them with a single node n and the names of all of them.
func newInsertDeleteList(t *testing.T) (head, tail, p, a, y, n *elist.ListHead, names map[*elist.ListHead]string) {
	t.Helper()
	entries := make([]typedEntry, 6)
	head, tail = &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y, n = &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	names = map[*elist.ListHead]string{head: "head", p: "p", a: "a", y: "y", n: "n", tail: "tail"}
	return
}

// alone returns an error unless n links only to itself.
func alone(names map[*elist.ListHead]string, n *elist.ListHead) error {
	if next, prev := n.DirectNext(), n.DirectPrev(); next != n || prev != n {
		return fmt.Errorf("%s is not alone: next %s, prev %s", names[n], names[next], names[prev])
	}
	return nil
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a passes the mark check of a and stops at "insert.begin". a is then
// deleted to its end, so p.next is y. The insert resumes: it reads p as the
// node before a, and its first CAS, p.next from a to n, fails on every one of
// its 100 tries. InsertBefore drops that error and returns nil although n is
// not linked. The insert must return an error, and n must stay alone.
func TestInsertBeforeNodeMarkedBeforeFirstCAS(t *testing.T) {
	head, tail, p, a, y, n, names := newInsertDeleteList(t)
	_, _ = p, y

	s := newStepper(t)
	ins := s.stopAt("insert.begin", n)
	doneI, errI := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)
	if err := a.MarkForDelete(); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	ins.Release()
	waitClosed(t, doneI, "insert n")

	assertLinked(t, names, head, tail, "p", "y")
	if err := alone(names, n); err != nil {
		t.Error(err)
	}
	if *errI == nil {
		t.Error("InsertBefore(n) returned no error, but n is not linked")
	}
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a passes the mark check of a and stops at "insert.begin". a is then
// deleted with Delete, which marks it, unlinks it and Inits it, so a links
// only to itself. The insert resumes: the node before a is now a itself, and
// both CASes of listAddWitCas(n, a, a) succeed. InsertBefore returns nil, and
// n and a make a ring outside the list. The insert must return an error, and
// n must stay alone.
func TestInsertBeforeNodeInitedBeforeFirstCAS(t *testing.T) {
	head, tail, p, a, y, n, names := newInsertDeleteList(t)
	_, _ = p, y

	s := newStepper(t)
	ins := s.stopAt("insert.begin", n)
	doneI, errI := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)
	if _, err := a.Delete(); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	ins.Release()
	waitClosed(t, doneI, "insert n")

	assertLinked(t, names, head, tail, "p", "y")
	if *errI == nil {
		t.Error("InsertBefore(n) returned no error, but n is not linked")
	}
	if err := alone(names, n); err != nil {
		t.Error(err)
	}
}
