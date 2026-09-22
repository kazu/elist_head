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
