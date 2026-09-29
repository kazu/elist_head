//go:build stephook

package elist_head_test

import (
	"testing"
	"time"
	"unsafe"

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

// purgeDo deletes n as Purge of skiplistmap does: it clears the links of n
// once the delete returns.
func purgeDo(n *elist.ListHead) (<-chan struct{}, *error) {
	return goDo(func() error {
		err := n.MarkForDelete()
		if err == nil {
			n.InitMarked()
		}
		return err
	})
}

// Nodes p, m, n and s lie in this order between head and tail, and are
// purged at once. Purging n stops after it marked n, and purging m stops
// after it led p to s, passing over n. Purging n must then wait for the
// purge of m, which gives way when it finds s marked by the purge of s: it
// led s back to p instead, relying on the link from p, and the purge of m
// then put that link back, leading p to m and m to n, whose links are gone.
func TestMarkForDeleteWaitsForTheDeleteThatTookItOut(t *testing.T) {
	entries := make([]typedEntry, 6)
	head, tail := &entries[0].ListHead, &entries[5].ListHead
	elist.InitAsEmpty(head, tail)
	p, m, n, s := &entries[1].ListHead, &entries[2].ListHead, &entries[3].ListHead, &entries[4].ListHead
	for _, x := range []*elist.ListHead{p, m, n, s} {
		if _, err := tail.InsertBefore(x); err != nil {
			t.Fatal(err)
		}
	}

	st := newStepper(t)
	delN := st.stopAt("del.marked", n)
	doneN, _ := purgeDo(n)
	delN.waitReached(t)
	delM := st.stopAt("del.prevRelinked", m)
	doneM, _ := purgeDo(m)
	delM.waitReached(t)
	delN.Release()
	select {
	case <-doneN:
	case <-time.After(200 * time.Millisecond):
	}
	delS := st.stopAt("del.marked", s)
	doneS, _ := purgeDo(s)
	delS.waitReached(t)
	delM.Release()
	waitClosed(t, doneM, "purge m")
	delS.Release()
	waitClosed(t, doneN, "purge n")
	waitClosed(t, doneS, "purge s")

	names := map[*elist.ListHead]string{head: "head", p: "p", m: "m", n: "n", s: "s", tail: "tail"}
	assertLinked(t, names, head, tail, "p")
}

// Nodes x, the slice [p s], q and z lie in this order between head and
// tail. Purging p and purging s stop after they marked their nodes, and
// purging q takes p, s and q out, passing over p and s. A move of the slice
// then starts. The purges of p and s must end: p and s are out of the list
// already. They gave way to the move instead, and the move waited for ever
// on p and s, which still link to each other.
func TestMarkForDeleteOfAMovedNodeTakenOutEnds(t *testing.T) {
	out := make([]typedEntry, 5)
	src, dst := make([]typedEntry, 2), make([]typedEntry, 2)
	head, x, q, z, tail := &out[0].ListHead, &out[1].ListHead, &out[2].ListHead, &out[3].ListHead, &out[4].ListHead
	p, s := &src[0].ListHead, &src[1].ListHead
	elist.InitAsEmpty(head, tail)
	for _, n := range []*elist.ListHead{x, p, s, q, z} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}

	st := newStepper(t)
	delP := st.stopAt("del.marked", p)
	doneP, errP := purgeDo(p)
	delP.waitReached(t)
	delS := st.stopAt("del.marked", s)
	doneS, errS := purgeDo(s)
	delS.waitReached(t)
	doneQ, errQ := purgeDo(q)
	waitClosed(t, doneQ, "purge q")
	if *errQ != nil {
		t.Fatalf("purge q: %v", *errQ)
	}
	marked := st.stopAt("move.marked", p)
	doneM, _ := goDo(func() error {
		mv := elist.FreezeSlice(
			unsafe.Pointer(&src[0]),
			unsafe.Pointer(&src[len(src)-1]),
			unsafe.Pointer(&dst[0]),
			int(unsafe.Sizeof(src[0])),
			int(unsafe.Offsetof(src[0].ListHead)))
		mv.Relink()
		return nil
	})
	marked.waitReached(t)
	marked.Release()
	delP.Release()
	delS.Release()
	waitClosed(t, doneP, "purge p")
	waitClosed(t, doneS, "purge s")
	waitClosed(t, doneM, "move")
	if *errP != nil || *errS != nil {
		t.Errorf("purge p, purge s = %v, %v, want nil, nil", *errP, *errS)
	}

	names := map[*elist.ListHead]string{head: "head", x: "x", p: "p", s: "s", q: "q", z: "z", tail: "tail"}
	assertLinked(t, names, head, tail, "x", "z")
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
