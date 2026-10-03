package elist_head_test

import (
	elist "github.com/kazu/elist_head"
	"testing"
)

func TestDeleteAndReinsert(t *testing.T) {
	nodes := make([]elist.ListHead, 4)
	elist.InitAsEmpty(&nodes[0], &nodes[3])
	for i := 1; i < 3; i++ {
		if _, err := nodes[3].InsertBefore(&nodes[i]); err != nil {
			t.Fatal(err)
		}
	}
	for n := 0; n < 10; n++ {
		if _, err := nodes[2].Delete(); err != nil {
			t.Fatal(err)
		}
		if nodes[1].DirectNext() != &nodes[3] || nodes[3].DirectPrev() != &nodes[1] {
			t.Fatal("unlink")
		}
		if _, err := nodes[3].InsertBefore(&nodes[2]); err != nil {
			t.Fatal(err)
		}
	}
}

// Nodes x, a, b and y lie in this order between head and tail. After a and
// then b are deleted, no node links to either of them, so both are safe to
// reuse, though the links of a and b still point to each other.
func TestIsSafetyAfterAdjacentDeletes(t *testing.T) {
	nodes := make([]elist.ListHead, 6)
	elist.InitAsEmpty(&nodes[0], &nodes[5])
	for i := 1; i < 5; i++ {
		if _, err := nodes[5].InsertBefore(&nodes[i]); err != nil {
			t.Fatal(err)
		}
	}
	a, b := &nodes[2], &nodes[3]
	for _, n := range []*elist.ListHead{a, b} {
		if err := n.MarkForDelete(); err != nil {
			t.Fatal(err)
		}
	}
	for name, n := range map[string]*elist.ListHead{"a": a, "b": b} {
		if safe, _ := n.IsSafety(); !safe {
			t.Errorf("deleted %s is not safe to reuse", name)
		}
	}
}
