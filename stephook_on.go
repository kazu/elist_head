//go:build stephook

package elist_head

import "sync/atomic"

// StepHook is called at named points of insertion and deletion when the
// package is built with the stephook tag, so that a test can stop a goroutine
// there and replay a concurrent interleaving one step at a time.
//
// Points and arguments:
//   - "insert.begin" (new, nil, next): InsertBefore is called.
//   - "add.cas1" (new, prev, next): before prev.next is changed to new.
//   - "add.cas2" (new, prev, next): prev.next is new; before next.prev is changed.
//   - "add.rollback" (new, prev, next): next.prev was not changed; before prev.next is put back.
//   - "del.begin" (node, prev, next): MarkForDelete read the links of node; before they are marked.
//   - "del.nextMarked" (node, prev, next): node.next is marked; before node.prev is marked.
//   - "del.marked" (node, prev, next): both links of node are marked.
//   - "del.relink" (node, prev, next): prev.next was node; before it is changed to next.
//   - "del.check" (node, prev, next): the links to node are changed; before they are checked.
//   - "repair.prevLinked" (old, prev, copy): RepaireSliceAfterCopy changed prev.next from
//     old to its copy; before the links of the copy are moved.
//   - "replace.begin" (head, nextHead, next): ReplaceNext starts a try; before
//     head.next and next.prev are read.
//   - "replace.read" (head, nextHead, next): head.next and next.prev are read;
//     before any of them is changed.
//   - "replace.cas2" (head, nextHead, next): head.next is nextHead; before
//     next.prev is changed.
//   - "replace.retry" (head, nextHead, next): the CAS of head.next failed and
//     nothing was changed; before the next try.
//   - "replace.rollback" (head, nextHead, next): the CAS of next.prev failed
//     and head.next is put back; before the next try.
type StepHook func(point string, a, b, c *ListHead)

var stepHook atomic.Pointer[StepHook]

// SetStepHook installs fn, or removes the hook when fn is nil.
func SetStepHook(fn StepHook) {
	if fn == nil {
		stepHook.Store(nil)
		return
	}
	stepHook.Store(&fn)
}

// LinkMarks reports whether the prev link and the next link of n carry the
// mark of a delete, so that a test sees a mark that a later store or CAS
// took away while IsMarked still sees the other one.
func LinkMarks(n *ListHead) (prev, next bool) {
	return atomic.LoadUintptr(&n.prev)&1 != 0, atomic.LoadUintptr(&n.next)&1 != 0
}

func stepAt(point string, a, b, c *ListHead) {
	if fn := stepHook.Load(); fn != nil {
		(*fn)(point, a, b, c)
	}
}
