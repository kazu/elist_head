//go:build stephook

package elist_head

import (
	"testing"
	"time"
)

func TestReplaceWithAdjacentReplacement(t *testing.T) {
	storage := make([]ListHead, 6)
	head, tail, left, right := &storage[0], &storage[1], &storage[2], &storage[3]
	freshLeft, freshRight := &storage[4], &storage[5]
	InitAsEmpty(head, tail)
	if _, err := tail.InsertBefore(left); err != nil {
		t.Fatal(err)
	}
	if _, err := tail.InsertBefore(right); err != nil {
		t.Fatal(err)
	}
	leftReady, rightReady := make(chan struct{}), make(chan struct{})
	releaseLeft, releaseRight := make(chan struct{}), make(chan struct{})
	release := func(ch chan struct{}) {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	defer release(releaseLeft)
	defer release(releaseRight)
	SetStepHook(func(point string, a, b, c *ListHead) {
		if point == "replaceNode.mark" && a == left {
			close(leftReady)
			<-releaseLeft
		}
		if point == "replaceNode.marked" && a == right {
			close(rightReady)
			<-releaseRight
		}
	})
	defer SetStepHook(nil)
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("replacement did not reach step")
		}
	}
	leftDone, rightDone := make(chan error, 1), make(chan error, 1)
	go func() { leftDone <- left.ReplaceWith(freshLeft) }()
	wait(leftReady)
	go func() { rightDone <- right.ReplaceWith(freshRight) }()
	wait(rightReady)
	release(releaseLeft)
	select {
	case err := <-leftDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("left replacement did not finish")
	}
	release(releaseRight)
	select {
	case err := <-rightDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("right replacement did not finish")
	}
	nodes := []*ListHead{head, freshLeft, freshRight, tail}
	for i := 0; i < len(nodes)-1; i++ {
		if nodes[i].DirectNext() != nodes[i+1] || nodes[i+1].DirectPrev() != nodes[i] {
			t.Fatalf("broken adjacency at %d", i)
		}
	}
}
