// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"errors"
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
// nodes from sHead to sTail. The caller stops every writer that changes a
// link of a node of the slice or of a node next to it until the repair
// ends. When a link outside the slice changed anyway, it returns an error and
// leaves the links it moved so far as they are.
//
//go:nocheckptr
func RepaireSliceAfterCopy(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) error {

	start := uintptr(sHead)
	last := uintptr(sTail) + uintptr(size)

	moved := int(uintptr(dHead)) - int(uintptr(sHead))

	// outer reports whether the link v of the node at ptr leaves the slice
	outer := func(ptr, v uintptr) bool {
		return ptr+(v&^1) < start || ptr+(v&^1) > last
	}
	// a link of a node outside the slice that is moved to the copy
	type outerLink struct {
		link     *uintptr
		old, new uintptr
		prevSide bool
		src, t   *ListHead
	}
	var links []outerLink

	// write the links of the copy from the links the source has now, before
	// any node outside the slice leads to the copy
	for cur := unsafe.Add(sHead, offset); uintptr(cur) < uintptr(last); cur = unsafe.Add(cur, size) {
		src := (*ListHead)(cur)
		dst := (*ListHead)(unsafe.Add(cur, moved))
		prev, next := atomic.LoadUintptr(&src.prev), atomic.LoadUintptr(&src.next)
		if outer(uintptr(cur), prev) {
			t := (*ListHead)(unsafe.Add(cur, int(prev&^1)))
			links = append(links, outerLink{link: &t.next, old: uintptr(cur) - uintptr(unsafe.Pointer(t)), prevSide: true, src: src, t: t})
			prev = IncPointer(prev, -moved)
		}
		if outer(uintptr(cur), next) {
			t := (*ListHead)(unsafe.Add(cur, int(next&^1)))
			links = append(links, outerLink{link: &t.prev, old: uintptr(cur) - uintptr(unsafe.Pointer(t)), src: src, t: t})
			next = IncPointer(next, -moved)
		}
		atomic.StoreUintptr(&dst.prev, prev)
		atomic.StoreUintptr(&dst.next, next)
	}

	// lead the nodes outside the slice to the copy
	for i := range links {
		l := &links[i]
		l.new = IncPointer(l.old, moved)
		if !Cas(l.link, l.old, l.new) {
			return errors.New("duplicated rewrite outside ListHead")
		}
		if l.prevSide {
			stepAt("repair.prevLinked", l.src, l.t, (*ListHead)(unsafe.Add(unsafe.Pointer(l.src), moved)))
		}
	}
	return nil
}
