//go:build stephook

package elist_head_test

import (
	"strings"
	"testing"

	elist "github.com/kazu/elist_head"
)

// walkI5 returns the names of the nodes from head toward tail, walking forward
// when forward is true and backward from tail otherwise. It stops after
// len(names)+1 steps, so a walk caught in a loop ends with "...".
func walkI5(names map[*elist.ListHead]string, head, tail *elist.ListHead, forward bool) []string {
	var got []string
	cur, end := head.DirectNext(), tail
	if !forward {
		cur, end = tail.DirectPrev(), head
	}
	for cur != end {
		if len(got) > len(names) {
			return append(got, "...")
		}
		got = append(got, names[cur])
		if forward {
			cur = cur.DirectNext()
		} else {
			cur = cur.DirectPrev()
		}
	}
	if !forward {
		for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
			got[i], got[j] = got[j], got[i]
		}
	}
	return got
}

// Nodes p, a and y lie in this order between head and tail, and n is alone.
// Inserting n before y reads a as the node before y, makes its first CAS,
// a.next from y to n, and stops at "add.cas2", before its second CAS, y.prev
// from a to n. So a.next is n, n.prev is a and n.next is y, but y.prev is
// still a. a is then deleted with Delete in another goroutine, and the insert
// resumes. Delete must not initialize a while y.prev points to it. When both
// end, y is deleted, and the list must hold p and not a or y, walking forward
// and backward, with n either linked or alone.
func TestReproI5DeleteInitsNodeStillPrevOfNext(t *testing.T) {
	head, tail, p, a, y, n, names := newInsertDeleteList(t)
	_ = p

	s := newStepper(t)
	ins := s.stopAt("add.cas2", n)
	doneI, errI := goDo(func() error { _, err := y.InsertBefore(n); return err })
	ins.waitReached(t)
	if got := a.DirectNext(); got != n {
		t.Fatalf("after the first CAS a.next is %s, want n", names[got])
	}

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
	if y.DirectPrev() == a {
		t.Errorf("Delete initialized a while y.prev still points to it: y.prev %s, a.next %s, a.prev %s",
			names[y.DirectPrev()], names[a.DirectNext()], names[a.DirectPrev()])
	}
	if err := y.MarkForDelete(); err != nil {
		t.Errorf("delete y: %v", err)
	}
	if got := tail.DirectPrev(); got == a {
		t.Errorf("after deleting y, tail.prev is the initialized a")
	}
	t.Logf("insert n returned %v", *errI)

	fwd, bwd := walkI5(names, head, tail, true), walkI5(names, head, tail, false)
	joined := " " + strings.Join(fwd, " ") + " "
	switch {
	case strings.Join(fwd, " ") != strings.Join(bwd, " "):
		t.Errorf("forward %q, backward %q differ", fwd, bwd)
	case !strings.Contains(joined, " p "):
		t.Errorf("forward %q lacks p", fwd)
	case strings.Contains(joined, " a ") || strings.Contains(joined, " y "):
		t.Errorf("forward %q holds a deleted node", fwd)
	case !strings.Contains(joined, " n "):
		if err := alone(names, n); err != nil {
			t.Error(err)
		}
	}
}
