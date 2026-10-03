//go:build stephook

package elist_head

import "sync/atomic"

// StepHook is called at named points of insertion and deletion when the
// package is built with the stephook tag, so that a test can stop a goroutine
// there and replay a concurrent interleaving one step at a time.
//
// Points and arguments:
//   - "insert.begin" (new, nil, next): InsertBefore is called.
//   - "insert.safe" (new, nil, next): TryInsertBefore found new marked, not moved, and IsSafety of new true; before it clears the marked links of new.
//   - "add.cas1" (new, prev, next): before prev.next is changed to new.
//   - "add.cas2" (new, prev, next): prev.next is new; before next.prev is changed.
//   - "add.rollback" (new, prev, next): next.prev was not changed, or a move marked new; before prev.next is put back to next.
//   - "del.begin" (node, prev, next): MarkForDelete read the links of node; before they are marked.
//   - "del.nextMarked" (node, prev, next): node.next is marked; before node.prev is marked.
//   - "del.marked" (node, prev, next): both links of node are marked.
//   - "del.nextRead" (node, prev, next): prev.next leads to node, and MarkForDelete read next, the node after node to change prev.next to; before it looks for a move.
//   - "del.relink" (node, prev, next): prev.next was node; before it is changed to next.
//   - "del.prevRelinked" (node, prev, next): prev.next was changed from node to next; before MarkForDelete looks for a move.
//   - "del.prevChecked" (node, prev, next): prev.next was changed from node to next, and MarkForDelete found no move; before it checks the links to node again.
//   - "del.beforeRead" (node, prev, next): next.prev leads to node, and MarkForDelete read prev, the node whose next is next, to change next.prev to; before it looks for a move.
//   - "del.nextRelink" (node, prev, next): next.prev was node; before it is changed to prev.
//   - "del.nextRelinked" (node, prev, next): next.prev was changed from node to prev; before MarkForDelete looks for a move.
//   - "del.check" (node, prev, next): the links to node are changed; before they are checked.
//   - "move.marked" (first, nil, nil): FreezeSlice marked both links of every
//     node of the slice, first is its first node; before it reads the links.
//   - "move.wait" (old, node, nil): FreezeSlice found an insert or a delete
//     next to old between its CASes, or Relink found that node, outside the
//     slice, does not lead to old; before it reads the links again.
//   - "move.written" (first, nil, nil): the links and the data of the copies
//     are written; before Relink leads the nodes outside the slice to them.
//   - "move.waitWritten" (old, node, nil): Relink found that node, next to
//     old, is a node of another slice that is moved; before it reads again
//     whether the copy of node is written.
//   - "move.waitDone" (node, nil, nil): MovedTo found the slice of node, whose
//     move is not done; before it reads again whether it is.
//   - "origin.loaded" (node, nil, nil): FindOrigin read the list of the moves
//     whose slices are kept; before it looks node up in that list.
//   - "del.waitWritten" (node, nil, nil): MarkForDelete took node out of the
//     list and found the slice of node, whose copies are not written; before
//     it reads again whether they are.
//   - "repair.prevLinked" (old, prev, copy): Relink changed prev.next from
//     old to its copy, after the links of every copy were written; before it
//     moves the next link from a node outside the slice.
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
