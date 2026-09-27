// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"errors"
	"runtime"
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

	// lead the nodes outside the slice to the copy, and put them back to the
	// source when one of them no longer links to the source
	for i := range links {
		l := &links[i]
		l.new = IncPointer(l.old, moved)
		if !Cas(l.link, l.old, l.new) {
			for j := i - 1; j >= 0; j-- {
				putBackToSource(links[j].src, links[j].t, moved, links[j].prevSide)
			}
			return errors.New("duplicated rewrite outside ListHead")
		}
		if l.prevSide {
			stepAt("repair.prevLinked", l.src, l.t, (*ListHead)(unsafe.Add(unsafe.Pointer(l.src), moved)))
		}
	}
	return nil
}

// putBackToSource leads the list back from the copy of src to src, on the
// side of prev when prevSide is true and of next otherwise. The node outside
// the slice that leads to the copy may differ from the one the repair moved,
// since other writers insert and delete there meanwhile: it waits until the
// node that the copy links to on that side links back to the copy, and moves
// that link.
//
//go:nocheckptr
func putBackToSource(src, outer *ListHead, moved int, prevSide bool) {
	dst := (*ListHead)(unsafe.Add(unsafe.Pointer(src), moved))
	// src.next is written only here and by an insert after src, which then
	// waits for the prev link of the node after src to be src
	last := uintptr(unsafe.Pointer(outer)) - uintptr(unsafe.Pointer(src))
	for {
		if prevSide {
			p := (*ListHead)(unsafe.Add(unsafe.Pointer(dst), int(atomic.LoadUintptr(&dst.prev)&^1)))
			atomic.StoreUintptr(&src.prev, uintptr(unsafe.Pointer(p))-uintptr(unsafe.Pointer(src)))
			if Cas(&p.next, uintptr(unsafe.Pointer(dst))-uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(src))-uintptr(unsafe.Pointer(p))) {
				stepAt("repair.putBack", src, p, dst)
				return
			}
		} else {
			n := (*ListHead)(unsafe.Add(unsafe.Pointer(dst), int(atomic.LoadUintptr(&dst.next)&^1)))
			// n is outer unless nodes were inserted after the copy or n
			// was deleted: then lead src to n as the first CAS of an insert
			// of n does, and wait while an insert after src holds src.next
			if want := uintptr(unsafe.Pointer(n)) - uintptr(unsafe.Pointer(src)); want != last {
				if !Cas(&src.next, last, want) {
					runtime.Gosched()
					continue
				}
				last = want
			}
			if Cas(&n.prev, uintptr(unsafe.Pointer(dst))-uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(src))-uintptr(unsafe.Pointer(n))) {
				stepAt("repair.putBack", src, n, dst)
				return
			}
		}
		runtime.Gosched()
	}
}
