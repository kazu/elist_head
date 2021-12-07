// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

var sharedModeTraverse *list_head.ModeTraverse = list_head.NewTraverse()

func RollbacksharedModeTraverse(prev list_head.TravOpt) {
	sharedModeTraverse.Option(prev)
}

func StoreListHead(dst *unsafe.Pointer, src *ListHead) {
	atomic.StorePointer(dst,
		unsafe.Pointer(src))
}
func Cas(target *unsafe.Pointer, old, new *ListHead) bool {
	return atomic.CompareAndSwapPointer(target,
		unsafe.Pointer(old),
		unsafe.Pointer(new))
}

func MarkListHead(target *unsafe.Pointer, old unsafe.Pointer) bool {

	//mask := uintptr(^uint(0)) ^ 1
	return atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(target)),
		*target,
		unsafe.Pointer(uintptr(old)|1))

}

func (head *ListHead) noInners(start, end uintptr) (result []unsafe.Pointer) {

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

func RepaireSliceAfterCopy(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) {

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
				t.next = unsafe.Add(t.next, moved)
				dHead.prev = unsafe.Add(dHead.prev, -moved)
				tt := dHead.directPrev()
				succ := tt == t && tt.directNext() != cHead
				if !succ {
					fmt.Printf("invalid prev")
				}

			} else if iPtr == cHead.next {
				t := cHead.directNext()
				t.prev = unsafe.Add(t.prev, moved)
				dHead.next = unsafe.Add(dHead.next, -moved)
				tt := dHead.directNext()
				succ := tt == t && tt.directPrev() != cHead
				if !succ {
					fmt.Printf("invalid next")
				}
			}
		}
		cntChanged++
	}

}
