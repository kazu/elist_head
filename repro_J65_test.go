//go:build stephook

package elist_head_test

import (
	"testing"
)

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a stops at "add.cas2" after its first CAS: p.next is n, n.prev is
// p, n.next is a, and a.prev is still p. a is then deleted with Delete in
// another goroutine, and the insert resumes. The delete must not finish, and
// so a must not be Inited and inserted again, while n still links to a: if
// it did, a.prev could hold p again and the second CAS of the insert, a.prev
// from p to n, would pass on this ABA and make a and n a ring. When both end,
// the list must hold p and y without a ring, the insert must return an
// error, and n must be alone.
func TestInsertBeforeSecondCASPassesOnReinsertedNode(t *testing.T) {
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

	if *errI == nil {
		t.Error("InsertBefore(n) returned no error, but a was deleted")
	}
	if err := wantLinked(names, head, tail, "p", "y"); err != nil {
		t.Error(err)
	}
	if err := alone(names, n); err != nil {
		t.Error(err)
	}
}
