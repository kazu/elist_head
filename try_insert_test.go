package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

func TestTryInsertBeforeLinksWhenAccepted(t *testing.T) {
	entries := make([]typedEntry, 3)
	head, tail := &entries[0].ListHead, &entries[2].ListHead
	elist.InitAsEmpty(head, tail)
	var got *elist.ListHead
	err := tail.TryInsertBefore(&entries[1].ListHead, func(prev *elist.ListHead) bool {
		got = prev
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != head {
		t.Errorf("accept got %p, want the head %p", got, head)
	}
	if head.DirectNext() != &entries[1].ListHead || tail.DirectPrev() != &entries[1].ListHead {
		t.Error("new node is not linked between head and tail")
	}
}

func TestTryInsertBeforeLeavesListWhenRejected(t *testing.T) {
	entries := make([]typedEntry, 3)
	head, tail := &entries[0].ListHead, &entries[2].ListHead
	elist.InitAsEmpty(head, tail)
	err := tail.TryInsertBefore(&entries[1].ListHead, func(prev *elist.ListHead) bool { return false })
	if err == nil {
		t.Fatal("rejected insertion returned no error")
	}
	if head.DirectNext() != tail || tail.DirectPrev() != head || !entries[1].IsSingle() {
		t.Error("rejected insertion changed the links")
	}
}

func TestTryInsertBeforeFailsBeforeUnlinkedNode(t *testing.T) {
	entries := make([]typedEntry, 2)
	err := entries[0].TryInsertBefore(&entries[1].ListHead, func(prev *elist.ListHead) bool { return true })
	if err == nil {
		t.Fatal("insertion before a node that is not linked returned no error")
	}
	if !entries[0].IsSingle() || !entries[1].IsSingle() {
		t.Error("failed insertion changed the links")
	}
}
