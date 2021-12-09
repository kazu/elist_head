package elist_head

import (
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"

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

	if uintptr(head.next)&1 > 0 {
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
		if opt(head) != nil {
			break
		}
	}
	return nil, nil
}

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

	mask := uintptr(^uint(0)) ^ 1

	var (
		ErrDeketeStep0 error = errors.New("fail step 0")
		ErrDeketeStep1 error = errors.New("fail step 1")
		ErrDeketeStep2 error = errors.New("fail step 2")
		ErrDeketeStep3 error = errors.New("fail step 3")
	)
	_, _ = ErrDeketeStep2, ErrDeketeStep3

	err = list_head.Retry(100, func(retry int) (fin bool, err error) {
		prev1 := (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.prev)&mask)))
		next1 := (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.next)&mask)))

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

		if retry > 50 {
			fmt.Printf("retry > 50\n")

		}

		if !MarkListHead(&head.next, uintptr(head.diffPtrToHead(next))) {
			//		if !MarkListHead(&l.next, unsafe.Pointer(next)) {
			//AddRecoverState("remove: retry marked next")
			return false, ErrDeketeStep0
		}
		if !MarkListHead(&head.prev, uintptr(head.diffPtrToHead(prev))) {
			//if !MarkListHead(&l.prev, unsafe.Pointer(prev)) {

			//AddRecoverState("remove: retry marked prev")
			return false, ErrDeketeStep1
		}
		if !prev1.Empty() {
			// mode.Mu(prev1).Lock()
			// defer mode.Mu(prev1).Unlock()
		}
		prev2 := PrevNoM(head)
		next2 := NextNoM(head)
		if mode.Mu != nil {
			if !prev2.Empty() {
				// mode.Mu(prev2).Lock()
				// defer mode.Mu(prev2).Unlock()
			}
			if !next2.Empty() {
				// mode.Mu(next2).Lock()
				// defer mode.Mu(next2).Unlock()
			}
		}

		_, _ = prev2, next2
		prevs := [2]*ListHead{prev1, prev2}
		nexts := [2]*ListHead{next1, next2}

		prevNexts := []*uintptr{&prev1.next, &prev2.next}
		nextPrevs := []*uintptr{&next1.prev, &next2.prev}
		//		nexts := []**ListHead{&next1.prev, &next2.prev}

		// prevs := []**ListHead{&prev1.next, &prev2.next}
		// nexts := []**ListHead{&next1.prev, &next2.prev}

		t := false
		_ = t
		for i, pn := range prevNexts {
			// prev1 := (*ListHead)(unsafe.Add(l.ptr(), int(uintptr(l.prev)&mask)))
			//  *pn != l
			if unsafe.Add(unsafe.Pointer(prevs[i]), int(uintptr(*pn))) != unsafe.Pointer(head) {
				continue
			}

			next := next1
			if next.IsMarked() {
				next = next2
			}
			t = Cas(prevNexts[i], (*ListHead)(prevs[i].diffPtrToHead(head)), (*ListHead)(prevs[i].diffPtrToHead(next)))
			//t = Cas(prevNexts[i], l, next)
		}

		for i, np := range nextPrevs {
			//_ = i
			if unsafe.Add(unsafe.Pointer(nexts[i]), int(uintptr(*np))) != unsafe.Pointer(head) {
				continue
			}

			prev := prev1
			if prev.IsMarked() {
				prev = prev2
			}

			t = Cas(np, (*ListHead)(nexts[i].diffPtrToHead(head)), (*ListHead)(nexts[i].diffPtrToHead(prev)))
			//t = Cas(np, l, prev)
		}
		errs := []error{}

		for i, toL := range append(prevNexts, nextPrevs...) {
			_ = i
			var base *ListHead
			if i < 2 {
				base = prevs[i%2]
			} else {
				base = nexts[i%2]
			}
			a := unsafe.Add(unsafe.Pointer(base), int(uintptr(*toL)))
			b := unsafe.Pointer(head)
			_, _ = a, b

			if unsafe.Add(unsafe.Pointer(base), int(uintptr(*toL))) == unsafe.Pointer(head) {
				//return false, ErrDeketeStep2
				errs = append(errs, ErrDeketeStep2)

			} else {
				errs = append(errs, nil)
			}
			// if l == *toL {
			// 	//AddRecoverState("remove: found node to me")
			// 	return false, ErrDeketeStep2
			// }
		}

		prev2 = PrevNoM(head)
		next2 = NextNoM(head)
		return true, nil
	})

	if err != nil {
		mode.SetError(err)
	}

	return err
}

func PrevNoM(head *ListHead) *ListHead {

	prev := uintptr(head.prev)
	mask := uintptr(^uint(0)) ^ 1
	if uintptr(prev)&1 == 0 {
		return head.directPrev()
	}

	pHead := (*ListHead)(unsafe.Add(head.ptr(), int(prev&mask)))

	return PrevNoM(pHead)

}

func NextNoM(head *ListHead) *ListHead {
	next := uintptr(head.next)
	mask := uintptr(^uint(0)) ^ 1
	if uintptr(next)&1 == 0 {
		return head.directNext()
	}

	nHead := (*ListHead)(unsafe.Add(head.ptr(), int(next&mask)))

	return NextNoM(nHead)
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
			head.prev, head.next = uintptr(0), uintptr(0)
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
