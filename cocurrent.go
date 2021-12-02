// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"sync/atomic"
	"unsafe"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

var sharedModeTraverse *list_head.ModeTraverse = list_head.NewTraverse()

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
