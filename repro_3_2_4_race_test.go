//go:build stephook && race

package elist_head_test

import (
	"runtime"
	"testing"

	elist "github.com/kazu/elist_head"
)

// Nodes p, a and q lie in this order between head and tail. p.ReplaceNext,
// which replaces a with the chain n1, n2, stops at "replace.begin" before it
// reads p.next and q.prev. Deleting a stops at "del.relink" before it changes
// p.next from a to q. The hook is removed, so that the lock in the stepper
// does not order the two, and on the only P the delete runs to its end: it
// changes p.next and q.prev with CAS. The replace is released after that
// without waiting for the delete, and reads p.next and q.prev with plain
// loads, which is a data race with those CASes. These plain loads are the
// values that the rollback of ReplaceNext stores back.
func TestReplaceNextPlainReadRace(t *testing.T) {
	head, tail, nodes, n1, n2, names := newReplaceList(t, "p", "a", "q")
	p, a, q := nodes[0], nodes[1], nodes[2]

	s := newStepper(t)
	begin := s.stopAt("replace.begin", p)
	doneR, errR := goDo(func() error { return p.ReplaceNext(n1, n2, q) })
	begin.waitReached(t)
	relink := s.stopAt("del.relink", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	relink.waitReached(t)

	elist.SetStepHook(nil)
	procs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(procs) })
	relink.Release()
	runtime.Gosched()
	begin.Release()

	waitClosed(t, doneD, "delete a")
	waitClosed(t, doneR, "replace a")
	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}
	if *errR != nil {
		t.Fatalf("replace a: %v", *errR)
	}
	assertLinked(t, names, head, tail, "p", "n1", "n2", "q")
}
