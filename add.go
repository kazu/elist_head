package elist_head

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

//   prev.next -> new
//   prev      <- new.prev
//   new.next  -> head
//   new       <- head.prev

func (head *ListHead) __InsertBefore(new *ListHead) {

	//prev.next, head.prev = prev.diffPtrToHead(head), head.diffPtrToHead(prev)

	prev := head.directPrev()

	prev.next, new.prev, new.next = uintptr(prev.diffPtrToHead(new)), uintptr(new.diffPtrToHead(prev)), uintptr(new.diffPtrToHead(head))

}

// func (head *ListHead) InsertBefore(new Head, opts ...list_head.TravOpt) (Head, error) {
// 	nhead := new.(*ListHead)

// 	return head._InsertBefore(nhead, opts...)
// }

func (head *ListHead) InsertBefore(new *ListHead, opts ...list_head.TravOpt) (*ListHead, error) {

	//prev := head.directPrev()

	if new.IsMarked() {
		if ok, _ := new.IsSafety(); ok {
			atomic.StoreUintptr(&new.prev, 0)
			atomic.StoreUintptr(&new.next, 0)
		} else {
			return head, ErrNoSafetyOnAdd
		}
	}

	if head.isMarkedForDeleteWithoutError() {
		return head, ErrMarked
	}
	stepAt("insert.begin", new, nil, head)

	nNode := toNode(new)
	err := head.insertBefore(nNode, opts...)
	return head, err


}

// TryInsertBefore links new just before head in one attempt. It reads the
// node before head once and links new there only if accept returns true for
// that node. It returns an error without linking new when head is marked, is
// not linked to a previous node, is rejected by accept, when new is linked,
// or when another goroutine changed the links first; the caller finds the
// position again.
func (head *ListHead) TryInsertBefore(new *ListHead, accept func(prev *ListHead) bool) error {
	return head.TryInsertBeforeTaking(new, accept, nil)
}

// TryInsertBeforeTaking links new as TryInsertBefore does, and calls took,
// when it is not nil, each time this insert takes new, before it links new or
// puts new back. While one insert holds new, no other insert takes it.
func (head *ListHead) TryInsertBeforeTaking(new *ListHead, accept func(prev *ListHead) bool, took func()) error {

	if new.IsMarked() {
		if findMoving(new) != nil {
			return ErrMoved
		}
		if ok, _ := new.IsSafety(); ok {
			stepAt("insert.safe", new, nil, head)
			// only while the marks stay: another insert of new may have
			// cleared and taken the links already
			new.InitMarked()
		} else {
			return ErrNoSafetyOnAdd
		}
	}

	if head.isMarkedForDeleteWithoutError() {
		return ErrMarked
	}
	stepAt("insert.begin", new, nil, head)

	prev := head.directPrev()
	if prev == head || !accept(prev) {
		return ErrNotAppend
	}
	return listAddTaking(toNode(new), prev, head, nil, took)
}

func (head *ListHead) insertBefore(new *ListHead, opts ...list_head.TravOpt) error {

	var err error
	mode := list_head.NewTraverse()
	defer mode.Error()
	for _, opt := range opts {
		opt(mode)
	}

	if !new.IsSingle() {
		mode.SetError(errors.New("Warn: elist_head ListHead.insert element must be single node"))
	}

	next := head
	prev := head.directPrev()
	err = list_head.Retry(100, func(retry int) (finish bool, err error) {
		if prev == head {
			return true, ErrNotAppend
		}
		err = listAddWitCas(new,
			prev,
			next, nil)
		//next, mode.Mu)
		if err == nil {
			return true, err
		}
		if head.isMarkedForDeleteWithoutError() {
			return true, ErrMarked
		}
		prev = head.directPrev()
		//AddRecoverState("cas retry")
		return false, err
	})
	if err != nil {
		mode.SetError(err)
	}
	return err
}

// ReplaceNext ... replace next element to new multiple list
func (head *ListHead) ReplaceNext(nextHead *ListHead, nextTail *ListHead, next *ListHead) (err error) {

	// MENTION: use mode, enable this
	// mode := list_head.NewTraverse()
	// defer mode.Error()
	// for _, opt := range opts {
	// 	opt(mode)
	// }

	// if !new.IsSingle() {
	// 	mode.SetError(errors.New("Warn: insert element must be single node"))
	// }

	// next := head
	// prev := head.directPrev()

	err = list_head.Retry(100, func(retry int) (finish bool, err error) {
		stepAt("replace.begin", head, nextHead, next)
		oldNext := atomic.LoadUintptr(&head.next)
		oldNewNextPrev := atomic.LoadUintptr(&next.prev)
		stepAt("replace.read", head, nextHead, next)

		// a delete of head or next has marked the link
		if oldNext&1 != 0 || oldNewNextPrev&1 != 0 {
			return true, ErrMarked
		}
		atomic.StoreUintptr(&nextHead.prev, uintptr(nextHead.diffPtrToHead(head)))
		atomic.StoreUintptr(&nextTail.next, uintptr(nextTail.diffPtrToHead(next)))

		if !Cas(&head.next, oldNext, uintptr(head.diffPtrToHead(nextHead))) {
			stepAt("replace.retry", head, nextHead, next)
			return false, NewError(ErrTCasConflictOnAdd, errors.New("cas conflict in Replace"))
		}
		stepAt("replace.cas2", head, nextHead, next)

		if !Cas(&next.prev, oldNewNextPrev, uintptr(next.diffPtrToHead(nextTail))) {
			Cas(&head.next, uintptr(head.diffPtrToHead(nextHead)), oldNext)
			stepAt("replace.rollback", head, nextHead, next)
			return false, NewError(ErrTCasConflictOnAdd, errors.New("cas conflict in Replace"))
		}

		return true, err
	})
	if err != nil {
		//mode.SetError(err)
	}
	return
}

type mutex struct {
	sync.Mutex
	enable bool
}

func newMutex(t bool) *mutex {
	return &mutex{enable: t}
}

func (mu *mutex) Lock() {
	if !mu.enable {
		return
	}
	mu.Mutex.Lock()
}

func (mu *mutex) Unlock() {
	if !mu.enable {
		return
	}
	mu.Mutex.Unlock()
}

var mu4Add *mutex = newMutex(false)

//  prev ---------------> next
//        \--> new --/
//   prev --> next     prev ---> new
func listAddWitCas(new, prev, next *ListHead, fn func(*ListHead) *sync.RWMutex) (err error) {
	return listAddTaking(new, prev, next, fn, nil)
}

// listAddTaking links new as listAddWitCas does, and calls took, when it is
// not nil, after it took the links of new and before it links new.
func listAddTaking(new, prev, next *ListHead, fn func(*ListHead) *sync.RWMutex, took func()) (err error) {
	// backup for roolback
	oNewPrev := atomic.LoadUintptr(&new.prev)
	oNewNext := atomic.LoadUintptr(&new.next)
	if (oNewPrev|oNewNext)&1 != 0 && findMoving(new) != nil {
		return ErrMoved
	}
	// new is taken from no links by the CAS below, so that of two inserts
	// of new only one links it; the other finds new linked
	if oNewPrev != 0 || oNewNext != 0 {
		return NewError(ErrTCasConflictOnAdd,
			fmt.Errorf("listAddWithCas() new is linked: new=%s prev=%s next=%s", new.P(), prev.P(), next.P()))
	}
	toPrev := uintptr(new.diffPtrToHead(prev))
	toNext := uintptr(new.diffPtrToHead(next))
	if fn != nil {
		if !prev.Empty() {
			fn(prev).Lock()
			defer fn(prev).Unlock()
		}
		if !next.Empty() {
			fn(next).Lock()
			defer fn(next).Unlock()
		}
	}
	// a link of new that a move marked meanwhile stays as it is
	rollback := func(new *ListHead) {
		Cas(&new.prev, toPrev, oNewPrev)
		Cas(&new.next, toNext, oNewNext)

		// StoreListHead(&new.prev, (*ListHead)(unsafe.Pointer(oNewPrev)))
		// StoreListHead(&new.next, (*ListHead)(unsafe.Pointer(oNewNext)))
	}
	_ = rollback

	// new.prev -> prev, new.next -> next; new.prev that another insert took
	// first is not put back
	if !Cas(&new.prev, oNewPrev, toPrev) {
		if IsMoved(new) {
			return ErrMoved
		}
		return NewError(ErrTCasConflictOnAdd,
			fmt.Errorf("listAddWithCas() the links of new changed: new=%s prev=%s next=%s", new.P(), prev.P(), next.P()))
	}
	if !Cas(&new.next, oNewNext, toNext) {
		rollback(new)
		if IsMoved(new) {
			return ErrMoved
		}
		return NewError(ErrTCasConflictOnAdd,
			fmt.Errorf("listAddWithCas() the links of new changed: new=%s prev=%s next=%s", new.P(), prev.P(), next.P()))
	}
	if took != nil {
		took()
	}
	// StoreListHead(&new.prev, prev)
	// StoreListHead(&new.next, next)

	// prev is deleted or moved: the CAS of prev.next fails on the mark,
	// until the caller finds another position
	if atomic.LoadUintptr(&prev.next)&1 != 0 {
		rollback(new)
		if IsMoved(new) {
			return ErrMoved
		}
		return ErrMarked
	}

	mu4Add.Lock()
	defer mu4Add.Unlock()
	a := prev.diffPtrToHead(next)
	b := prev.diffPtrToHead(new)
	_, _ = a, b
	stepAt("add.cas1", new, prev, next)
	// an insert before next waits while next is half inserted: the node after
	// next does not link back to next yet
	if nn := next.directNext(); nn != next && nn.directPrev() != next {
		goto ROLLBACK
	}
	if !Cas(&prev.next, uintptr(prev.diffPtrToHead(next)), uintptr(prev.diffPtrToHead(new))) {
		goto ROLLBACK
	}
	stepAt("add.cas2", new, prev, next)
	// a move marked new, and may have taken it as not linked
	if IsMoved(new) || !Cas(&next.prev, uintptr(next.diffPtrToHead(prev)), uintptr(next.diffPtrToHead(new))) {
		//if !Cas(&next.prev, prev, new) {

		stepAt("add.rollback", new, prev, next)
		undoLink(&prev.next, uintptr(prev.diffPtrToHead(new)), uintptr(prev.diffPtrToHead(next)))

		goto ROLLBACK

	}

	return nil

ROLLBACK:

	rollback(new)
	if IsMoved(new) {
		return ErrMoved
	}
	return NewError(ErrTCasConflictOnAdd,
		fmt.Errorf("listAddWithCas() please retry: new=%s prev=%s next=%s", new.P(), prev.P(), next.P()))

}

func (head *ListHead) IsMarked() bool {

	if atomic.LoadUintptr(&head.prev)&1 > 0 {
		return true
	}
	if atomic.LoadUintptr(&head.next)&1 > 0 {
		return true
	}
	return false
}

func (head *ListHead) IsSafety() (bool, error) {

	prev := PrevNoM(head) // should skip mark
	next := NextNoM(head) // should skip mark

	if prev.directNext().IsMarked() {
		return false, nil
	}
	if next.directPrev().IsMarked() {
		return false, nil
	}
	if prev == head {
		return false, nil
	}
	if next == head {
		return false, nil
	}
	if prev.directNext() == head || next.directPrev() == head {
		return false, nil
	}
	if !head.unlinked() {
		return false, nil
	}
	return true, nil

}
