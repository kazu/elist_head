//go:build stephook

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// TryInsertBefore takes a deleted node from its marks once. G1 inserts n,
// which a delete left marked and safe to reuse, before t1, and stops before
// it clears the links of n; G2 inserts n before t2 and returns; G1 goes on.
// G1 must fail, and n must stay where G2 linked it. G1 stored 0 into the
// links that G2 wrote, and linked n before t1 too.
func TestTryInsertBeforeTakesAMarkedNodeOnce(t *testing.T) {
	e := make([]typedEntry, 5)
	h1, t1, h2, t2, n := &e[0].ListHead, &e[1].ListHead, &e[2].ListHead, &e[3].ListHead, &e[4].ListHead
	names := map[*elist.ListHead]string{h1: "h1", t1: "t1", h2: "h2", t2: "t2", n: "n"}
	elist.InitAsEmpty(h1, t1)
	elist.InitAsEmpty(h2, t2)
	if _, err := t1.InsertBefore(n); err != nil {
		t.Fatal(err)
	}
	if err := n.MarkForDelete(); err != nil {
		t.Fatal(err)
	}
	any := func(*elist.ListHead) bool { return true }

	s := newStepper(t)
	st := s.stopAt("insert.safe", n)
	done1, err1 := goDo(func() error { return t1.TryInsertBefore(n, any) })
	st.waitReached(t)
	if err := t2.TryInsertBefore(n, any); err != nil {
		t.Fatalf("TryInsertBefore(n) of G2: %v", err)
	}
	st.Release()
	waitClosed(t, done1, "TryInsertBefore(n) of G1")

	if *err1 == nil {
		t.Errorf("TryInsertBefore(n) of G1 returned nil, but G2 linked n")
	}
	assertLinked(t, names, h1, t1)
	assertLinked(t, names, h2, t2, "n")
}
