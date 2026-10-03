//go:build stephook

package elist_head_test

import (
	"runtime"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// mvtwoList is a list that holds the nodes of two slices A and B, which two
// moves replace with their copies at the same time.
type mvtwoList struct {
	out        []typedEntry
	srcA, dstA []typedEntry
	srcB, dstB []typedEntry
	head, tail *elist.ListHead
	byNode     map[*elist.ListHead]string
	node       map[string]*elist.ListHead
}

// mvtwoNewList links the nodes named by order between a head and a tail. The
// names a1, a2 are the nodes of A, b1, b2 those of B, and every other name
// is a node outside the slices.
func mvtwoNewList(t *testing.T, order ...string) *mvtwoList {
	t.Helper()
	l := &mvtwoList{
		out:    make([]typedEntry, len(order)+2),
		srcA:   make([]typedEntry, 2),
		dstA:   make([]typedEntry, 2),
		srcB:   make([]typedEntry, 2),
		dstB:   make([]typedEntry, 2),
		byNode: map[*elist.ListHead]string{},
		node:   map[string]*elist.ListHead{},
	}
	l.head, l.tail = &l.out[0].ListHead, &l.out[1].ListHead
	l.byNode[l.head], l.byNode[l.tail] = "head", "tail"
	elist.InitAsEmpty(l.head, l.tail)
	for i, name := range []string{"a1", "a2"} {
		l.node[name], l.node[name+"'"] = &l.srcA[i].ListHead, &l.dstA[i].ListHead
	}
	for i, name := range []string{"b1", "b2"} {
		l.node[name], l.node[name+"'"] = &l.srcB[i].ListHead, &l.dstB[i].ListHead
	}
	next := 2
	for _, name := range order {
		if _, ok := l.node[name]; !ok {
			l.node[name] = &l.out[next].ListHead
			next++
		}
	}
	for name, n := range l.node {
		l.byNode[n] = name
	}
	for _, name := range order {
		linkAll(t, l.tail, l.node[name])
	}
	return l
}

// moveOp moves the slice src to dst.
func (l *mvtwoList) moveOp(name string, src, dst []typedEntry) op {
	nodes := make([]*elist.ListHead, 0, len(src))
	for i := range src {
		nodes = append(nodes, &src[i].ListHead)
	}
	return op{name: name, nodes: nodes, do: func() error {
		mv := elist.FreezeSlice(
			unsafe.Pointer(&src[0]),
			unsafe.Pointer(&src[len(src)-1]),
			unsafe.Pointer(&dst[0]),
			int(unsafe.Sizeof(src[0])),
			int(unsafe.Offsetof(src[0].ListHead)))
		for i := range src {
			if mv.Linked(i) {
				dst[i].Name, dst[i].Value = src[i].Name, src[i].Value
			}
		}
		mv.Relink()
		return nil
	}}
}

// exploreTwoMoves runs the moves of A and B at the same time in every
// schedule, on a list that holds the nodes of order, and wants the copies of
// the nodes of the slices in their place after each schedule.
func exploreTwoMoves(t *testing.T, order ...string) {
	want := make([]string, len(order))
	for i, name := range order {
		want[i] = name
		if name[0] == 'a' || name[0] == 'b' {
			want[i] = name + "'"
		}
	}
	explore(t, func() ([]op, func(errs []error) error) {
		l := mvtwoNewList(t, order...)
		ops := []op{
			l.moveOp("move A", l.srcA, l.dstA),
			l.moveOp("move B", l.srcB, l.dstB),
		}
		return ops, func(errs []error) error {
			defer runtime.KeepAlive(l)
			for _, err := range errs {
				if err != nil {
					return err
				}
			}
			return wantLinked(l.byNode, l.head, l.tail, want...)
		}
	})
}

// The last node of A and the first node of B are neighbors: each move leads
// the copy of the other slice to its own copy.
func TestMoveTwoSlicesNextToEachOther(t *testing.T) {
	exploreTwoMoves(t, "x", "a1", "a2", "b1", "b2", "y")
}

// The nodes of A and B take turns in the list: every link of a node of a
// slice leads to a node of the other slice.
func TestMoveTwoSlicesTakingTurns(t *testing.T) {
	exploreTwoMoves(t, "a1", "b1", "a2", "b2")
}

// A node outside the slices lies between A and B: the moves do not meet.
func TestMoveTwoSlicesApart(t *testing.T) {
	exploreTwoMoves(t, "a1", "a2", "x", "b1", "b2")
}
