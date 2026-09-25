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

// stopAt stops the first goroutine that reaches point with node as its first
// argument. reached is closed when it stops; release lets it go on.
func stopAt(t *testing.T, point string, node *elist.ListHead) (reached <-chan struct{}, release func()) {
	t.Helper()
	r, rel := make(chan struct{}), make(chan struct{})
	var stopOnce, releaseOnce sync.Once
	elist.SetStepHook(func(p string, a, b, c *elist.ListHead) {
		if p != point || a != node {
			return
		}
		stop := false
		stopOnce.Do(func() { stop = true })
		if !stop {
			return
		}
		close(r)
		<-rel
	})
	release = func() { releaseOnce.Do(func() { close(rel) }) }
	t.Cleanup(func() {
		elist.SetStepHook(nil)
		release()
	})
	return r, release
}

func waitClosed(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
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

	reached, release := stopAt(t, "del.marked", a)
	done := make(chan struct{})
	var errA error
	go func() {
		defer close(done)
		errA = a.MarkForDelete()
	}()
	waitClosed(t, reached, "a marking its links")
	if err := b.MarkForDelete(); err != nil {
		t.Fatalf("delete b: %v", err)
	}
	release()
	waitClosed(t, done, "delete a")
	if errA != nil {
		t.Fatalf("delete a: %v", errA)
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

	reached, release := stopAt(t, "del.marked", a)
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.MarkForDelete()
	}()
	waitClosed(t, reached, "a marking its links")
	prevs := elist.SharedTrav(list_head.Trav(list_head.TravSkipMark))
	next, prev := x.Next(), y.Prev()
	elist.SharedTrav(prevs...)
	release()
	waitClosed(t, done, "delete a")

	if next != y {
		t.Errorf("x.Next() skipping marks = %p, want y %p", next, y)
	}
	if prev != x {
		t.Errorf("y.Prev() skipping marks = %p, want x %p", prev, x)
	}
}
