package elist_head

import (
	"errors"
	"runtime"
	"sync/atomic"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

type BoolAndError struct {
	t bool
	e error
}

func MakeBoolAndError(t bool, e error) BoolAndError {
	return BoolAndError{t: t, e: e}
}

func (head *ListHead) isMarkedForDeleteWithoutError() (marked bool) {

	return MakeBoolAndError(head.isMarkedForDelete()).t
}

func (head *ListHead) isMarkedForDelete() (marked bool, err error) {

	if head == nil {
		return false, ErrListNil
	}
	//next := //atomic.LoadPointer((*unsafe.Pointer)(&head.next))
	next := head.directNext()

	if next == nil {
		return false, errors.New("next is nil")
	}

	if atomic.LoadUintptr(&head.next)&1 > 0 {
		return true, nil
	}
	return false, nil
}

func (head *ListHead) Delete(opts ...func(*ListHead) error) (result *ListHead, e error) {

	err := head.MarkForDelete()
	if err != nil {
		return nil, err
	}
	mu4Add.Lock()
	defer mu4Add.Unlock()
	if len(opts) == 0 {
		opts = append(opts, InitAfterSafety(100))
	}
	for _, opt := range opts {
		if e = opt(head); e != nil {
			break
		}
	}
	return nil, e
}

//go:nocheckptr
func (head *ListHead) MarkForDelete(opts ...list_head.TravOpt) (err error) {

	mode := list_head.NewTraverse()
	defer mode.Error()
	for _, opt := range opts {
		opt(mode)
	}

	if !head.canPurge() {
		return ErrNotMarked
	}
	mu4Add.Lock()
	defer mu4Add.Unlock()

	var (
		ErrDeketeStep0 error = errors.New("fail step 0")
		ErrDeketeStep1 error = errors.New("fail step 1")
		ErrDeketeStep2 error = errors.New("fail step 2")
		ErrDeketeStep3 error = errors.New("fail step 3")
	)
	_, _ = ErrDeketeStep2, ErrDeketeStep3

	try := func(retry int) (fin bool, err error) {
		prev1 := head.directPrev()
		next1 := head.directNext()
		stepAt("del.begin", head, prev1, next1)

		if mode.Mu != nil {
			// FIXME: later enable
			// if !prev1.Empty() {
			// 	mode.Mu(prev1).Lock()
			// 	defer mode.Mu(prev1).Unlock()
			// }
			// mode.Mu(l).Lock()
			// defer mode.Mu(l).Unlock()
			// if !next1.Empty() {
			// 	mode.Mu(next1).Lock()
			// 	defer mode.Mu(next1).Unlock()
			// }
		}

		prev := prev1
		next := next1

		if retry > 0 {
			// a neighbor is in the middle of an insert or a delete
			runtime.Gosched()
		}

		if !MarkListHead(&head.next, uintptr(head.diffPtrToHead(next))) {
			//		if !MarkListHead(&l.next, unsafe.Pointer(next)) {
			//AddRecoverState("remove: retry marked next")
			return false, ErrDeketeStep0
		}
		stepAt("del.nextMarked", head, prev1, next1)
		if !MarkListHead(&head.prev, uintptr(head.diffPtrToHead(prev))) {
			//if !MarkListHead(&l.prev, unsafe.Pointer(prev)) {

			//AddRecoverState("remove: retry marked prev")
			return false, ErrDeketeStep1
		}
		stepAt("del.marked", head, prev1, next1)
		if !prev1.Empty() {
			// mode.Mu(prev1).Lock()
			// defer mode.Mu(prev1).Unlock()
		}

		// relink the links to head from the nearest nodes that are not
		// marked, passing the marked nodes between them and head. An insert
		// that has made only its first CAS next to head is waited for: it
		// either finishes or removes its node.
		if halfInsertedBefore(head) {
			return false, ErrDeketeStep2
		}
		if x, v := linkingPrev(head); x != nil && v&1 == 0 {
			next := NextNoM(head)
			stepAt("del.relink", head, x, next)
			Cas(&x.next, v, uintptr(x.diffPtrToHead(next)))
		}
		if halfInsertedAfter(head) {
			return false, ErrDeketeStep2
		}
		if z, v := linkingNext(head); z != nil && v&1 == 0 {
			Cas(&z.prev, v, uintptr(z.diffPtrToHead(linkedBefore(head, z))))
		}
		stepAt("del.check", head, prev1, next1)
		if !head.unlinked() {
			return false, ErrDeketeStep2
		}

		return true, nil
	}
	for retry := 0; ; retry++ {
		if fin, e := try(retry); fin {
			err = e
			break
		}
	}

	return err
}

// PrevNoM returns the nearest node before head that is not marked, passing
// over the marked nodes between them.
//
//go:nocheckptr
func PrevNoM(head *ListHead) *ListHead {

	prev := head.directPrev()
	if prev == head || !prev.IsMarked() {
		return prev
	}
	return PrevNoM(prev)

}

// NextNoM returns the nearest node after head that is not marked, passing
// over the marked nodes between them.
//
//go:nocheckptr
func NextNoM(head *ListHead) *ListHead {
	next := head.directNext()
	if next == head || !next.IsMarked() {
		return next
	}
	return NextNoM(next)
}

func (head *ListHead) canPurge() bool {

	if head.directPrev() == head {
		return false
	}

	if head.directNext() == head {
		return false
	}
	return true
}

func InitAfterSafety(retry int) func(*ListHead) error {

	return func(head *ListHead) error {
		return list_head.Retry(retry, func(c int) (exit bool, err error) {
			if ok, _ := head.IsSafety(); !ok {
				return false, ErrNoSafetyOnAdd
			}
			atomic.StoreUintptr(&head.prev, 0)
			atomic.StoreUintptr(&head.next, 0)
			return true, nil
		})
	}

}

func IncPointer(t uintptr, moved int) uintptr {

	tOld := int(t)
	tOld += moved
	//	atomic.StorePointer(t, unsafe.Pointer(uintptr(tOld)))
	return uintptr(tOld)
}

func CasIncPointer(t *uintptr, same uintptr, moved int) bool {

	// tOld := int(t)
	// tOld += moved
	// return uintptr(tOld)

	return atomic.CompareAndSwapUintptr(t, same, uintptr(int(same)+moved))

}

// linkingPrev returns the nearest node before head that is not marked and
// the value of its next, when that next leads to head passing only marked
// nodes. It returns nil when no such link is left.
//
//go:nocheckptr
func linkingPrev(head *ListHead) (*ListHead, uintptr) {
	x := PrevNoM(head)
	v := atomic.LoadUintptr(&x.next)
	for cur := x.directNext(); cur != x; cur = cur.directNext() {
		if cur == head {
			return x, v
		}
		if !cur.IsMarked() || cur.directNext() == cur {
			break
		}
	}
	return nil, 0
}

// linkingNext returns the nearest node after head that is not marked and
// the value of its prev, when that prev leads to head passing only marked
// nodes. It returns nil when no such link is left.
//
//go:nocheckptr
func linkingNext(head *ListHead) (*ListHead, uintptr) {
	z := NextNoM(head)
	v := atomic.LoadUintptr(&z.prev)
	for cur := z.directPrev(); cur != z; cur = cur.directPrev() {
		if cur == head {
			return z, v
		}
		if !cur.IsMarked() || cur.directPrev() == cur {
			break
		}
	}
	return nil, 0
}

// halfInsertedBefore reports whether a node that is not marked links to
// head by its next while the node before it does not link to head: an
// insert before head has made its first CAS and not its second.
//
//go:nocheckptr
func halfInsertedBefore(head *ListHead) bool {
	x := PrevNoM(head)
	for cur := x.directNext(); cur != x && cur != head; cur = cur.directNext() {
		if cur.directNext() == cur {
			return false
		}
		if !cur.IsMarked() {
			return cur.directNext() == head
		}
	}
	return false
}

// halfInsertedAfter reports whether the nearest node after head that is not
// marked is linked from head while the node after it still links back to
// head: an insert after head has made its first CAS and not its second.
//
//go:nocheckptr
func halfInsertedAfter(head *ListHead) bool {
	z := NextNoM(head)
	if z == head || z.directNext() == z {
		return false
	}
	return z.directNext().directPrev() == head
}

// linkedBefore returns the node whose next is z, walking forward from the
// nearest node before head that is not marked, or that node when the walk
// does not reach z.
//
//go:nocheckptr
func linkedBefore(head, z *ListHead) *ListHead {
	x := PrevNoM(head)
	for cur := x; cur.directNext() != cur; cur = cur.directNext() {
		if cur.directNext() == z {
			return cur
		}
		if cur.directNext() == x {
			break
		}
	}
	return x
}

// unlinked reports whether no link reaches head from the nearest nodes
// before and after it that are not marked, and no insert next to head is
// between its two CASes.
func (head *ListHead) unlinked() bool {
	if x, _ := linkingPrev(head); x != nil {
		return false
	}
	if z, _ := linkingNext(head); z != nil {
		return false
	}
	return !halfInsertedBefore(head) && !halfInsertedAfter(head)
}
