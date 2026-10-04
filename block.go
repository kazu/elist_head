package elist_head

import "sync/atomic"

// Block is a nonempty, bidirectionally linked range. Its nodes must stay alive
// and its interior must not be modified while an operation on the block runs.
// Other operations may insert or delete nodes outside the block.
type Block struct {
	First *ListHead
	Last  *ListHead
}

// InitCopiedFrom completes an unpublished copy of src as an independent block.
// The caller must have copied the interior links with the same node layout and
// spacing. The interiors of both blocks must remain fixed during the call.
// Only the boundary links are written; no nodes are traversed.
func (b Block) InitCopiedFrom(src Block) {
	if b.First != b.Last {
		atomic.StoreUintptr(&b.First.next, atomic.LoadUintptr(&src.First.next))
		atomic.StoreUintptr(&b.Last.prev, atomic.LoadUintptr(&src.Last.prev))
	}
	atomic.StoreUintptr(&b.First.prev, 0)
	atomic.StoreUintptr(&b.Last.next, 0)
}

// InsertBefore tries to insert an independent block before next. Before the
// call, First.prev points to First and Last.next points to Last. On contention
// before publication it returns an error and leaves the block independent;
// the caller finds the position again. A block returned by Delete can be used
// again. Concurrent operations on the same block are not allowed.
func (b Block) InsertBefore(next *ListHead) error {
	oldPrev := atomic.LoadUintptr(&b.First.prev)
	oldNext := atomic.LoadUintptr(&b.Last.next)
	if oldPrev&^1 != 0 || oldNext&^1 != 0 {
		return ErrNotAppend
	}
	if next.IsMarked() {
		return ErrMarked
	}
	prev := next.directPrev()
	if prev == next {
		return ErrNotAppend
	}
	toPrev := uintptr(b.First.diffPtrToHead(prev))
	toNext := uintptr(b.Last.diffPtrToHead(next))
	if !Cas(&b.First.prev, oldPrev, toPrev) {
		return ErrNotAppend
	}
	if !Cas(&b.Last.next, oldNext, toNext) {
		Cas(&b.First.prev, toPrev, oldPrev)
		return ErrNotAppend
	}
	stepAt("block.insert.cas1", b.First, prev, next)
	if nn := next.directNext(); (nn != next && nn.directPrev() != next) ||
		!Cas(&prev.next, uintptr(prev.diffPtrToHead(next)), uintptr(prev.diffPtrToHead(b.First))) {
		Cas(&b.First.prev, toPrev, oldPrev)
		Cas(&b.Last.next, toNext, oldNext)
		return ErrNotAppend
	}
	stepAt("block.insert.cas2", b.First, prev, next)
	from := uintptr(next.diffPtrToHead(prev))
	to := uintptr(next.diffPtrToHead(b.Last))
	for {
		v := atomic.LoadUintptr(&next.prev)
		if v&^1 != from || Cas(&next.prev, v, to|v&1) {
			return nil
		}
	}
}

// Delete detaches the whole block. The interior is fixed by the caller while
// the operation runs. On success, the internal links are restored and the
// outward links are marked self-links, so stale boundary operations fail.
func (b Block) Delete() error {
	if atomic.LoadUintptr(&b.First.prev)&^1 == 0 || atomic.LoadUintptr(&b.Last.next)&^1 == 0 {
		return ErrNotMarked
	}
	if b.First == b.Last {
		if err := b.First.MarkForDelete(); err != nil {
			return err
		}
		atomic.StoreUintptr(&b.First.prev, 1)
		atomic.StoreUintptr(&b.First.next, 1)
		return nil
	}
	insideNext := atomic.LoadUintptr(&b.First.next)
	insidePrev := atomic.LoadUintptr(&b.Last.prev)
	atomic.StoreUintptr(&b.First.next, uintptr(b.First.diffPtrToHead(b.Last)))
	stepAt("block.delete.forward", b.First, b.Last, nil)
	atomic.StoreUintptr(&b.Last.prev, uintptr(b.Last.diffPtrToHead(b.First)))
	stepAt("block.delete.compressed", b.First, b.Last, nil)
	if err := b.First.MarkForDelete(); err != nil {
		return err
	}
	stepAt("block.delete.first", b.First, b.Last, nil)
	if err := b.Last.MarkForDelete(); err != nil {
		return err
	}
	atomic.StoreUintptr(&b.First.next, insideNext)
	atomic.StoreUintptr(&b.Last.prev, insidePrev)
	atomic.StoreUintptr(&b.First.prev, 1)
	atomic.StoreUintptr(&b.Last.next, 1)
	return nil
}
