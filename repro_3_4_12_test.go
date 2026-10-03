//go:build stephook

package elist_head_test

import (
	"fmt"
	"strings"
	"testing"

	elist "github.com/kazu/elist_head"
)

// newRollbackList links p, a and y in this order between head and tail, and
// returns them with two single nodes n and m and the names of all of them.
func newRollbackList(t *testing.T) (head, tail, p, a, y, n, m *elist.ListHead, names map[*elist.ListHead]string) {
	t.Helper()
	entries := make([]typedEntry, 7)
	head, tail = &entries[0].ListHead, &entries[6].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y, n, m = &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead, &entries[5].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}
	names = map[*elist.ListHead]string{head: "head", p: "p", a: "a", y: "y", n: "n", m: "m", tail: "tail"}
	return
}

// checkRolledBackList returns an error unless the walk from head forward ends
// at tail, the walk from tail backward sees the same nodes, and every node
// that links forward to n is the node n links back to.
func checkRolledBackList(names map[*elist.ListHead]string, head, tail, n *elist.ListHead) error {
	var errs []string
	var fwd, bwd []string
	for cur := head.DirectNext(); cur != tail; cur = cur.DirectNext() {
		if len(fwd) > len(names) {
			errs = append(errs, fmt.Sprintf("forward walk does not end: %q", fwd))
			break
		}
		fwd = append(fwd, names[cur])
	}
	for cur := tail.DirectPrev(); cur != head; cur = cur.DirectPrev() {
		if len(bwd) > len(names) {
			errs = append(errs, fmt.Sprintf("backward walk does not end: %q", bwd))
			break
		}
		bwd = append([]string{names[cur]}, bwd...)
	}
	if strings.Join(fwd, " ") != strings.Join(bwd, " ") {
		errs = append(errs, fmt.Sprintf("forward %q, backward %q", fwd, bwd))
	}
	for x, name := range names {
		if x != n && x.DirectNext() == n && n.DirectPrev() != x {
			errs = append(errs, fmt.Sprintf("%s.next is n, but n.prev is %s and n.next is %s", name, names[n.DirectPrev()], names[n.DirectNext()]))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a stops at "add.cas2" after its first CAS: p.next is n, n links to
// p and a, and a.prev is still p. a is then deleted in another goroutine: it
// marks a and waits for the insert of n. The insert resumes: its second CAS
// fails on the mark of a.prev, and it stops at "add.rollback". Inserting m
// before n now must not link m, because n is half inserted: m would be left
// before a node that goes away. When the rollback resumes, it removes n as a
// delete does, the delete of a finishes, and the list must hold p and y with
// n and m alone.
func TestInsertBeforeRollbackAfterInsertBeforeNew(t *testing.T) {
	head, tail, _, a, _, n, m, names := newRollbackList(t)

	s := newStepper(t)
	cas2 := s.stopAt("add.cas2", n)
	doneI, errI := goDo(func() error { _, err := a.InsertBefore(n); return err })
	cas2.waitReached(t)
	marked := s.stopAt("del.marked", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	marked.waitReached(t)
	marked.Release()
	rb := s.stopAt("add.rollback", n)
	cas2.Release()
	rb.waitReached(t)
	if _, err := n.InsertBefore(m); err == nil {
		t.Errorf("inserting m before the half inserted n returned no error")
	}
	rb.Release()
	waitClosed(t, doneI, "insert n")
	waitClosed(t, doneD, "delete a")

	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}
	if *errI == nil {
		t.Error("InsertBefore(n) returned no error, but a was deleted")
	}
	if err := checkRolledBackList(names, head, tail, n); err != nil {
		t.Error(err)
	}
	if err := wantLinked(names, head, tail, "p", "y"); err != nil {
		t.Error(err)
	}
	for _, x := range []*elist.ListHead{n, m} {
		if x.DirectNext() != x || x.DirectPrev() != x {
			t.Errorf("%s is not alone: next %s, prev %s", names[x], names[x.DirectNext()], names[x.DirectPrev()])
		}
	}
}
