package elist_head

import (
	"unsafe"
)

type List interface {
	Offset() uintptr
	PtrListHead() *ListHead
	FromListHead(*ListHead) List
}

func __ElementOf(l List, head *ListHead) unsafe.Pointer {
	if head == nil || l == nil {
		return nil
	}

	return unsafe.Pointer(uintptr(head.Ptr()) - l.Offset())
}

func _ElementOf(l List, head *ListHead) unsafe.Pointer {
	if head == nil || l == nil {
		return nil
	}

	return unsafe.Pointer(uintptr(unsafe.Pointer(head)) - l.Offset())
}

func ElementOf(head unsafe.Pointer, offset uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(head) - offset)
}
