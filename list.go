package elist_head

import "unsafe"

// List provides typed operations over existing embedded ListHead links.
// It stores only the offset of the embedded field and retains no elements.
// The caller keeps elements alive, compares terminators, synchronizes mutations
// and repairs links after copying. Terminators traversed through typed operations
// must also be embedded in T, just as for data nodes.
type List[T any] struct {
	offset uintptr
}

// NewList configures typed operations without changing any links.
// offset must be unsafe.Offsetof of the ListHead field within T.
// Relative traversal retains ListHead's allocation limits.
func NewList[T any](offset uintptr) List[T] {
	return List[T]{offset: offset}
}

// Link returns the embedded link of a non-nil live element.
func (l List[T]) Link(v *T) *ListHead {
	return (*ListHead)(unsafe.Add(unsafe.Pointer(v), l.offset))
}

// Element returns the containing element, including a containing terminator.
// h must be the configured field of a non-nil live T.
func (l List[T]) Element(h *ListHead) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(h), -int(l.offset)))
}

func (l List[T]) Next(v *T) *T {
	h := (*ListHead)(unsafe.Add(unsafe.Pointer(v), l.offset)).Next()
	return (*T)(unsafe.Add(unsafe.Pointer(h), -int(l.offset)))
}

func (l List[T]) Prev(v *T) *T {
	h := (*ListHead)(unsafe.Add(unsafe.Pointer(v), l.offset)).Prev()
	return (*T)(unsafe.Add(unsafe.Pointer(h), -int(l.offset)))
}

func (l List[T]) DirectNext(v *T) *T {
	return l.Element(l.Link(v).DirectNext())
}

func (l List[T]) DirectPrev(v *T) *T {
	return l.Element(l.Link(v).DirectPrev())
}

func (l List[T]) InsertBefore(at, v *T) error {
	_, err := l.Link(at).InsertBefore(l.Link(v))
	return err
}
