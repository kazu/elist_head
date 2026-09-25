//go:build stephook

package elist_head_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	elist "github.com/kazu/elist_head"
	list_head "github.com/kazu/loncha/lista_encabezado"
)

// stepper stops goroutines at the step points of elist_head, so that a test
// replays a concurrent interleaving one step at a time.
type stepper struct {
	mu    sync.Mutex
	stops []*stepStop
}

type stepStop struct {
	point   string
	node    *elist.ListHead
	used    bool
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func newStepper(t *testing.T) *stepper {
	t.Helper()
	s := &stepper{}
	elist.SetStepHook(s.at)
	t.Cleanup(func() {
		elist.SetStepHook(nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, st := range s.stops {
			st.Release()
		}
	})
	return s
}

// stopAt stops the first goroutine that reaches point with node as its first
// argument, until Release is called.
func (s *stepper) stopAt(point string, node *elist.ListHead) *stepStop {
	st := &stepStop{point: point, node: node, reached: make(chan struct{}), release: make(chan struct{})}
	s.mu.Lock()
	s.stops = append(s.stops, st)
	s.mu.Unlock()
	return st
}

func (s *stepper) at(point string, a, b, c *elist.ListHead) {
	s.mu.Lock()
	var st *stepStop
	for _, x := range s.stops {
		if !x.used && x.point == point && x.node == a {
			x.used = true
			st = x
			break
		}
	}
	s.mu.Unlock()
	if st == nil {
		return
	}
	close(st.reached)
	<-st.release
}

func (st *stepStop) Release() {
	st.once.Do(func() { close(st.release) })
}

// waitReached fails the test unless a goroutine stops at st.
func (st *stepStop) waitReached(t *testing.T) {
	t.Helper()
	waitClosed(t, st.reached, st.point)
}

func waitClosed(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

// goDo runs fn in a new goroutine and returns a channel closed when fn
// returns, and a pointer to the error fn returned.
func goDo(fn func() error) (<-chan struct{}, *error) {
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		err = fn()
	}()
	return done, &err
}

// assertLinked checks that the list from head to tail holds exactly the nodes
// named in want, walking forward and walking backward.
func assertLinked(t *testing.T, names map[*elist.ListHead]string, head, tail *elist.ListHead, want ...string) {
	t.Helper()
	var fwd, bwd []string
	for cur := head.DirectNext(); cur != tail && len(fwd) <= len(names); cur = cur.DirectNext() {
		fwd = append(fwd, names[cur])
	}
	for cur := tail.DirectPrev(); cur != head && len(bwd) <= len(names); cur = cur.DirectPrev() {
		bwd = append([]string{names[cur]}, bwd...)
	}
	for _, got := range [][]string{fwd, bwd} {
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("forward %q, backward %q, want %q", fwd, bwd, want)
			return
		}
	}
}

// Nodes x, a, b and y lie in this order between head and tail. a and b are
// deleted at the same time: a stops after marking its links, b is deleted,
// and then a finishes. x and y must be linked to each other in both
// directions.
func TestMarkForDeleteAdjacentKeepsNeighbors(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	x, a, b, y := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, n := range []*elist.ListHead{x, a, b, y} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	stop := s.stopAt("del.marked", a)
	done, errA := goDo(func() error { return a.MarkForDelete() })
	stop.waitReached(t)
	if err := b.MarkForDelete(); err != nil {
		t.Fatalf("delete b: %v", err)
	}
	stop.Release()
	waitClosed(t, done, "delete a")
	if *errA != nil {
		t.Fatalf("delete a: %v", *errA)
	}

	names := map[*elist.ListHead]string{head: "head", x: "x", a: "a", b: "b", y: "y", tail: "tail"}
	assertLinked(t, names, head, tail, "x", "y")
}

// Nodes x, a and y lie in this order between head and tail. While a is being
// deleted and its links are marked, the SkipMark traversal from x and from y
// must pass over a.
func TestSkipMarkPassesMarkedNode(t *testing.T) {
	entries := make([]typedEntry, 5)
	head, tail := &entries[0].ListHead, &entries[4].ListHead
	elist.InitAsEmpty(head, tail)
	x, a, y := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead
	for _, n := range []*elist.ListHead{x, a, y} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	stop := s.stopAt("del.marked", a)
	done, _ := goDo(func() error { return a.MarkForDelete() })
	stop.waitReached(t)
	prevs := elist.SharedTrav(list_head.Trav(list_head.TravSkipMark))
	next, prev := x.Next(), y.Prev()
	elist.SharedTrav(prevs...)
	stop.Release()
	waitClosed(t, done, "delete a")

	if next != y {
		t.Errorf("x.Next() skipping marks = %p, want y %p", next, y)
	}
	if prev != x {
		t.Errorf("y.Prev() skipping marks = %p, want x %p", prev, x)
	}
}

// Nodes a and y lie in this order between head and tail. Deleting a stops
// after reading the links of a; n is then inserted between a and y. When the
// delete resumes, n must stay linked between head and y.
func TestMarkForDeleteKeepsInsertAfterNode(t *testing.T) {
	entries := make([]typedEntry, 5)
	head, tail := &entries[0].ListHead, &entries[4].ListHead
	elist.InitAsEmpty(head, tail)
	a, y, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead
	for _, x := range []*elist.ListHead{a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	stop := s.stopAt("del.begin", a)
	done, errA := goDo(func() error { return a.MarkForDelete() })
	stop.waitReached(t)
	if _, err := y.InsertBefore(n); err != nil {
		t.Fatalf("insert n: %v", err)
	}
	stop.Release()
	waitClosed(t, done, "delete a")
	if *errA != nil {
		t.Fatalf("delete a: %v", *errA)
	}

	names := map[*elist.ListHead]string{head: "head", a: "a", y: "y", n: "n", tail: "tail"}
	assertLinked(t, names, head, tail, "n", "y")
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a stops before its first CAS, and deleting a stops after marking
// a.next. The insert then links n between p and a, and the delete finishes.
// n must stay linked between p and y.
func TestMarkForDeleteKeepsInsertBeforeNode(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	ins := s.stopAt("add.cas1", n)
	doneI, errI := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)
	del := s.stopAt("del.nextMarked", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	del.waitReached(t)
	ins.Release()
	waitClosed(t, doneI, "insert n")
	if *errI != nil {
		t.Fatalf("insert n: %v", *errI)
	}
	del.Release()
	waitClosed(t, doneD, "delete a")
	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}

	names := map[*elist.ListHead]string{head: "head", p: "p", a: "a", y: "y", n: "n", tail: "tail"}
	assertLinked(t, names, head, tail, "p", "n", "y")
}

// Nodes p, a and y lie in this order between head and tail. Inserting n
// before a stops before its first CAS. Deleting a marks its links and stops
// before it changes p.next from a to y. The insert then changes p.next to n,
// fails on the mark of a.prev and stops before putting p.next back, so the
// delete fails to change p.next and stops before its check. When the insert
// has put p.next back to a and given up, and the delete finishes, p must be
// linked to y.
func TestMarkForDeleteRetriesWhileLinkedTo(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y, n := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	ins := s.stopAt("add.cas1", n)
	doneI, _ := goDo(func() error { _, err := a.InsertBefore(n); return err })
	ins.waitReached(t)
	relink := s.stopAt("del.relink", a)
	doneD, errD := goDo(func() error { return a.MarkForDelete() })
	relink.waitReached(t)
	rollback := s.stopAt("add.rollback", n)
	ins.Release()
	rollback.waitReached(t)
	check := s.stopAt("del.check", a)
	relink.Release()
	check.waitReached(t)
	rollback.Release()
	waitClosed(t, doneI, "insert n")
	check.Release()
	waitClosed(t, doneD, "delete a")
	if *errD != nil {
		t.Fatalf("delete a: %v", *errD)
	}

	names := map[*elist.ListHead]string{head: "head", p: "p", a: "a", y: "y", n: "n", tail: "tail"}
	assertLinked(t, names, head, tail, "p", "y")
}

// Nodes p, a and y lie in this order between head and tail. While a is being
// deleted and p and y still link to it, a is not safe to reuse; after the
// delete it is.
func TestIsSafetyWhileNeighborsLinkTo(t *testing.T) {
	entries := make([]typedEntry, 5)
	head, tail := &entries[0].ListHead, &entries[4].ListHead
	elist.InitAsEmpty(head, tail)
	p, a, y := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead
	for _, x := range []*elist.ListHead{p, a, y} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}

	s := newStepper(t)
	stop := s.stopAt("del.marked", a)
	done, errA := goDo(func() error { return a.MarkForDelete() })
	stop.waitReached(t)
	during, _ := a.IsSafety()
	stop.Release()
	waitClosed(t, done, "delete a")
	if *errA != nil {
		t.Fatalf("delete a: %v", *errA)
	}
	after, _ := a.IsSafety()

	if during {
		t.Error("IsSafety() = true while p and y link to a")
	}
	if !after {
		t.Error("IsSafety() = false after a is deleted")
	}
}
