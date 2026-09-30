package elist_head

import (
	"runtime"
	"sync/atomic"
)

// ReplaceWith replaces one linked node. The caller keeps both allocations alive
// and excludes deletion/replacement of either node until this operation returns.
// Neighbouring insertions and deletions use the existing mark/CAS protocol.
// Unlike ReplaceNext, this operation never replaces a stale interval.
func (old *ListHead) ReplaceWith(fresh *ListHead) error {
	pv, nv := atomic.LoadUintptr(&old.prev), atomic.LoadUintptr(&old.next)
	if (pv|nv)&1 != 0 || pv == 0 || nv == 0 {
		return ErrMarked
	}
	prev, next := old.directPrev(), old.directNext()
	if atomic.LoadUintptr(&prev.next) != prev.diffPtrToHead(old) ||
		atomic.LoadUintptr(&next.prev) != next.diffPtrToHead(old) {
		return ErrCasConflictOnMark
	}
	toPrev, toNext := fresh.diffPtrToHead(prev), fresh.diffPtrToHead(next)
	if !Cas(&fresh.prev, 0, toPrev) {
		return ErrNotAppend
	}
	if !Cas(&fresh.next, 0, toNext) {
		Cas(&fresh.prev, toPrev, 0)
		return ErrNotAppend
	}
	stepAt("replaceNode.mark", old, fresh, next)
	// Mark next first, as in deletion. Its successor is now the replacement:
	// a concurrent delete that skips old must keep fresh in the next chain.
	if !Cas(&old.next, nv, old.diffPtrToHead(fresh)|1) {
		Cas(&fresh.prev, toPrev, 0)
		Cas(&fresh.next, toNext, 0)
		return ErrCasConflictOnMark
	}
	for {
		pv = atomic.LoadUintptr(&old.prev)
		if pv&1 != 0 || Cas(&old.prev, pv, pv|1) {
			break
		}
	}
	// An insertion immediately before old can finish between the marks.
	// Do not overwrite a link another operation has already corrected.
	Cas(&fresh.prev, toPrev, fresh.diffPtrToHead(old.directPrev()))
	stepAt("replaceNode.marked", old, fresh, next)

	for {
		prev = fresh.directPrev()
		v := atomic.LoadUintptr(&prev.next)
		if v&^1 == prev.diffPtrToHead(old) {
			if !Cas(&prev.next, v, prev.diffPtrToHead(fresh)|v&1) {
				runtime.Gosched()
				continue
			}
		} else if v&^1 != prev.diffPtrToHead(fresh) {
			// A partial insert/delete owns this link. Like insertBefore,
			// reread the position instead of replacing what is there now.
			runtime.Gosched()
			continue
		}
		stepAt("replaceNode.cas2", old, fresh, next)
		next = fresh.directNext()
		v = atomic.LoadUintptr(&next.prev)
		if v&^1 == next.diffPtrToHead(fresh) {
			return nil
		}
		if v&^1 == next.diffPtrToHead(old) && Cas(&next.prev, v, next.diffPtrToHead(fresh)|v&1) {
			return nil
		}
		// Preserve marks and the published next chain during rollback.
		// old.next still leads to fresh; readers retry the marked old node.
		undoLink(&prev.next, prev.diffPtrToHead(fresh), prev.diffPtrToHead(old))
		stepAt("replaceNode.rollback", old, fresh, next)
		runtime.Gosched()
	}
}
