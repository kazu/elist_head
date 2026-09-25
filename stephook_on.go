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
//   - "del.check" (node, prev, next): the links to node are changed; before they are checked.
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

func stepAt(point string, a, b, c *ListHead) {
	if fn := stepHook.Load(); fn != nil {
		(*fn)(point, a, b, c)
	}
}
