// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"sync/atomic"
	"unsafe"

	list_head "github.com/kazu/lista_encabezado"
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

// MarkListHead sets the mark bit of the link at target only while the link
// still holds old. It also succeeds when the link already holds old with the
// mark bit, so that a retried delete can mark the same link again.
func MarkListHead(target *uintptr, old uintptr) bool {

	//mask := uintptr(^uint(0)) ^ 1
	if atomic.CompareAndSwapUintptr(target,
		old,
		uintptr(old)|1) {
		return true
	}
	return atomic.LoadUintptr(target) == uintptr(old)|1

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

// RepaireSliceAfterCopy leads the list to the copy at dHead of the slice of
// nodes from sHead to sTail, which the caller copied before. It is FreezeSlice
// and Relink of a SliceMove in one step, for a slice whose data no writer
// changes meanwhile.
func RepaireSliceAfterCopy(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) error {
	FreezeSlice(sHead, sTail, dHead, size, offset).Relink()
	return nil
}
