package elist_head

import (
	"unsafe"
)

// ElementOf returns the containing element at the supplied field offset.
// head must point to that field of a live element.
func ElementOf(head unsafe.Pointer, offset uintptr) unsafe.Pointer {
	return unsafe.Add(head, -int(offset))
}
