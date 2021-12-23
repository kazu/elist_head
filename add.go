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
			new.prev, new.next = uintptr(0), uintptr(0)
		} else {
			return head, ErrNoSafetyOnAdd
		}
	}

	if head.isMarkedForDeleteWithoutError() {
		return head, ErrMarked
	}

	nNode := toNode(new)
	head.insertBefore(nNode, opts...)
	return head, nil

	return nil, nil

}

func (head *ListHead) insertBefore(new *ListHead, opts ...list_head.TravOpt) {

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
		err = listAddWitCas(new,
			prev,
			next, nil)
		//next, mode.Mu)
		if err == nil {
			return true, err
		}
		prev = head.directPrev()
		//AddRecoverState("cas retry")
		return false, err
	})
	if err != nil {
		mode.SetError(err)
	}
	return
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
		oldNext := head.next
		oldNewNextPrev := next.prev

		rollback := func(head, next *ListHead) {
			atomic.StoreUintptr(&head.next, oldNext)
			atomic.StoreUintptr(&next.prev, oldNewNextPrev)
		}
		_ = rollback
		atomic.StoreUintptr(&nextHead.prev, uintptr(nextHead.diffPtrToHead(head)))
		atomic.StoreUintptr(&nextTail.next, uintptr(nextTail.diffPtrToHead(next)))

		if !Cas(&head.next, oldNext, uintptr(head.diffPtrToHead(nextHead))) {
			goto ROLLBACK
		}

		if !Cas(&next.prev, oldNewNextPrev, uintptr(next.diffPtrToHead(nextTail))) {
			goto ROLLBACK
		}

		return true, err

	ROLLBACK:
		rollback(head, next)
		return false, NewError(ErrTCasConflictOnAdd, errors.New("cas conflict in Replace"))
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
	// backup for roolback
	oNewPrev := new.prev
	oNewNext := new.next
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
	rollback := func(new *ListHead) {
		atomic.StoreUintptr(&new.prev, oNewPrev)
		atomic.StoreUintptr(&new.next, oNewNext)

		// StoreListHead(&new.prev, (*ListHead)(unsafe.Pointer(oNewPrev)))
		// StoreListHead(&new.next, (*ListHead)(unsafe.Pointer(oNewNext)))
	}
	_ = rollback

	// new.prev -> prev, new.next -> next
	atomic.StoreUintptr(&new.prev, uintptr(new.diffPtrToHead(prev)))
	atomic.StoreUintptr(&new.next, uintptr(new.diffPtrToHead(next)))
	// StoreListHead(&new.prev, prev)
	// StoreListHead(&new.next, next)

	mu4Add.Lock()
	defer mu4Add.Unlock()
	a := prev.diffPtrToHead(next)
	b := prev.diffPtrToHead(new)
	_, _ = a, b
	if !Cas(&prev.next, uintptr(prev.diffPtrToHead(next)), uintptr(prev.diffPtrToHead(new))) {
		goto ROLLBACK
	}
	if !Cas(&next.prev, uintptr(next.diffPtrToHead(prev)), uintptr(next.diffPtrToHead(new))) {
		//if !Cas(&next.prev, prev, new) {

		if !Cas(&prev.next, uintptr(prev.diffPtrToHead(new)), uintptr(prev.diffPtrToHead(next))) {
			//if !Cas(&prev.next, new, next) {
			_ = "fail rollback?"
		}

		goto ROLLBACK

	}

	return nil

ROLLBACK:

	rollback(new)
	return NewError(ErrTCasConflictOnAdd,
		fmt.Errorf("listAddWithCas() please retry: new=%s prev=%s next=%s", new.P(), prev.P(), next.P()))

}

func (head *ListHead) IsMarked() bool {

	if uintptr(head.prev)&1 > 0 {
		return true
	}
	if uintptr(head.next)&1 > 0 {
		return true
	}
	return false
}

func (head *ListHead) IsSafety() (bool, error) {

	prev := head.Prev() // should skip mark
	next := head.Next() // should skip mark

	if prev.IsMarked() {
		return false, nil
	}
	if next.IsMarked() {
		return false, nil
	}
	if prev == head {
		return false, nil
	}
	if next == head {
		return false, nil
	}
	return true, nil

}
