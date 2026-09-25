//go:build race

package elist_head_test

import (
	"testing"

	elist "github.com/kazu/elist_head"
)

// The tests in this file fail only through the race detector: each reads the
// links of nodes in one goroutine while an insertion stores them in another,
// with nothing ordering the two.

func TestIsMarkedDuringInsert(t *testing.T) {
	entries := make([]typedEntry, 3)
	head, tail := &entries[0].ListHead, &entries[2].ListHead
	elist.InitAsEmpty(head, tail)
	done := make(chan struct{})
	go func() {
		defer close(done)
		head.IsMarked()
		tail.IsMarked()
	}()
	if _, err := tail.InsertBefore(&entries[1].ListHead); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestNoMDuringInsert(t *testing.T) {
	entries := make([]typedEntry, 3)
	head, tail := &entries[0].ListHead, &entries[2].ListHead
	elist.InitAsEmpty(head, tail)
	done := make(chan struct{})
	go func() {
		defer close(done)
		elist.NextNoM(head)
		elist.PrevNoM(tail)
	}()
	if _, err := tail.InsertBefore(&entries[1].ListHead); err != nil {
		t.Fatal(err)
	}
	<-done
}
