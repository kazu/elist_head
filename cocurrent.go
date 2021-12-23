// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

var sharedModeTraverse *list_head.ModeTraverse = list_head.NewTraverse()

func RollbacksharedModeTraverse(prev list_head.TravOpt) {
	sharedModeTraverse.Option(prev)
}

func SharedTrav(travs ...list_head.TravOpt) []list_head.TravOpt {
	return sharedModeTraverse.Option(travs...)
}

func StoreListHead(dst *unsafe.Pointer, src *ListHead) {
	atomic.StorePointer(dst,
		unsafe.Pointer(src))
}
func _Cas(target *uintptr, old, new *ListHead) bool {
	return atomic.CompareAndSwapUintptr(target,
		uintptr(unsafe.Pointer(old)),
		uintptr(unsafe.Pointer(new)))
}

func Cas(target *uintptr, old, new uintptr) bool {
	return atomic.CompareAndSwapUintptr(target,
		old,
		new)
}

func MarkListHead(target *uintptr, old uintptr) bool {

	//mask := uintptr(^uint(0)) ^ 1
	return atomic.CompareAndSwapUintptr(target,
		*target,
		uintptr(old)|1)

}

func (head *ListHead) noInners(start, end uintptr) (result []uintptr) {

	ptr := uintptr(unsafe.Pointer(head))

	if ptr+uintptr(head.prev) < start || ptr+uintptr(head.prev) > end {
		result = append(result, head.prev)
	}

	if ptr+uintptr(head.next) < start || ptr+uintptr(head.next) > end {
		result = append(result, head.next)
	}

	return
}

func OuterPtrs(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) (outers []unsafe.Pointer) {

	start := uintptr(sHead)
	last := uintptr(sTail) + uintptr(size)

	// moved := int(uintptr(dHead)) - int(uintptr(sHead))

	// cntChanged := 0
	for cur := unsafe.Add(sHead, offset); uintptr(cur) < uintptr(last); cur = unsafe.Add(cur, size) {

		cHead := (*ListHead)(cur)
		ptrs := cHead.noInners(start, last)
		if len(ptrs) > 0 {
			outers = append(outers, cur)
		}
	}
	return
}

func RepaireSliceAfterCopy(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) error {

	start := uintptr(sHead)
	last := uintptr(sTail) + uintptr(size)

	moved := int(uintptr(dHead)) - int(uintptr(sHead))

	cntChanged := 0
	for cur := unsafe.Add(sHead, offset); uintptr(cur) < uintptr(last); cur = unsafe.Add(cur, size) {

		// if cntChanged >= 2 {
		// 	break
		// }

		cHead := (*ListHead)(cur)
		ptrs := cHead.noInners(start, last)

		if len(ptrs) == 0 {
			continue
		}
		dHead := (*ListHead)(unsafe.Add(cur, moved))

		// if cntChanged >= 3 {
		// 	fmt.Printf("invalid count")
		// }

		for _, iPtr := range ptrs {

			if iPtr == cHead.prev {
				t := cHead.directPrev()
				//t.next = IncPointer(t.next, moved)
				if !CasIncPointer(&t.next, uintptr(cur)-uintptr(unsafe.Pointer(t)), moved) {
					return errors.New("duplicated rewrite outside ListHead")
				}

				dHead.prev = IncPointer(dHead.prev, -moved)

				tt := dHead.directPrev()
				succ := tt == t && tt.directNext() != cHead
				if !succ {
					return fmt.Errorf("invalid ListHead.prev oldHead.direcvPrev()=%016p ?== newHead.directPrev()=%016p or newHead.directNext()=%016p ?== oldHead=%016p ",
						t, tt, tt.directNext(), cHead)
				}

			} else if iPtr == cHead.next {
				t := cHead.directNext()

				//t.prev = IncPointer(t.prev, moved)
				if !CasIncPointer(&t.prev, uintptr(cur)-uintptr(unsafe.Pointer(t)), moved) {
					return errors.New("duplicated rewrite outside ListHead")
				}
				dHead.next = IncPointer(dHead.next, -moved)

				tt := dHead.directNext()
				succ := tt == t && tt.directPrev() != cHead
				if !succ {
					return errors.New("invalid ListHead.next")
				}
			}
		}
		cntChanged++
	}
	return nil
}
