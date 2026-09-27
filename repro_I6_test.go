//go:build stephook

package elist_head_test

import (
	"testing"
	"time"
)

// Nodes p, a and y lie in this order between head and tail, and n is alone.
// Inserting n before a reads p as the node before a and stops at "add.cas1".
// a is then marked for deletion, and each try of MarkForDelete and each try
// of the insert take turns:
//
//  1. MarkForDelete marks both links of a (a mark already there counts as
//     done) and stops at "del.marked".
//  2. The insert makes its first CAS, p.next from a to n. Its second CAS,
//     a.prev from p to n, fails on the mark of a.prev, and it stops at
//     "add.rollback".
//  3. MarkForDelete skips p.next, which is n and not a, moves y.prev from a
//     to p on the first try only, and stops at "del.check".
//  4. The insert rolls p.next back from n to a. Its next try reads p again,
//     as the mark is taken off a.prev, and stops at "add.cas1".
//  5. MarkForDelete checks the links: p.next is a again, so the try fails
//     with step 2, and the next try stops at "del.marked".
//
// After 100 such rounds MarkForDelete gives up with ErrTOverRetyry and the
// insert uses up its 100 tries. a keeps both marks but is still p.next, so
// walking forward goes p a y while walking backward goes p y. MarkForDelete
// must not leave a marked node linked: the list must hold p and y, walking
// forward and backward.
func TestReproI6MarkForDeleteGivesUpLinked(t *testing.T) {
	head, tail, p, a, _, n, names := newInsertDeleteList(t)

	s := newStepper(t)
	ins := s.stopAt("add.cas1", n)
	doneI, _ := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)

	mk := s.stopAt("del.marked", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })

	rounds := 0
loop:
	for {
		select {
		case <-mk.reached:
		case <-doneD:
			break loop
		case <-time.After(10 * time.Second):
			t.Fatalf("delete a did not stop at del.marked in round %d", rounds+1)
		}
		select {
		case <-doneI:
			// The insert has no try left, so the delete runs alone.
			mk.Release()
			waitClosed(t, doneD, "delete a")
			break loop
		default:
		}
		rounds++

		rb := s.stopAt("add.rollback", n)
		ins.Release()
		rb.waitReached(t)

		ck := s.stopAt("del.check", a)
		mk.Release()
		ck.waitReached(t)

		ins = s.stopAt("add.cas1", n)
		rb.Release()
		select {
		case <-ins.reached:
		case <-doneI:
		case <-time.After(10 * time.Second):
			t.Fatalf("insert n neither tried again nor returned in round %d", rounds)
		}

		mk = s.stopAt("del.marked", a)
		ck.Release()
	}
	waitClosed(t, doneI, "insert n")
	t.Logf("rounds %d, MarkForDelete(a) returned %v", rounds, *errD)

	if *errD != nil && a.IsMarked() && p.DirectNext() == a {
		t.Errorf("MarkForDelete(a) gave up after %d rounds with %v, and a is marked but still p.next", rounds, *errD)
	}
	assertLinked(t, names, head, tail, "p", "y")
}
