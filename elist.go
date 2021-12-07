// Copyright 2019 Kazuhisa TAKEI<xtakei@rytr.jp>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package elist_head is like a kernel's LIST_HEAD
// usage for storing slice/array
package elist_head

import (
	"errors"
	"unsafe"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

var (
	MODE_CONCURRENT      bool = false
	PANIC_NEXT_IS_MARKED bool = false
)

const (
	ErrTDeleteFirst = 1 << iota
	ErrTListNil
	ErrTEmpty
	ErrTMarked
	ErrTNextMarked
	ErrTNotAppend
	ErrTNotMarked
	ErrTCasConflictOnMark
	ErrTFirstMarked
	ErrTCasConflictOnAdd
	ErrTOverRetyry
	ErrTNoSafety
	ErrTNoContinous
)

var (
	ErrDeleteFirst       error = NewError(ErrTDeleteFirst, errors.New("first element cannot delete"))
	ErrListNil           error = NewError(ErrTListNil, errors.New("list is nil"))
	ErrEmpty             error = NewError(ErrTEmpty, errors.New("list is empty"))
	ErrMarked            error = NewError(ErrTMarked, errors.New("element is marked"))
	ErrNextMarked        error = NewError(ErrTNextMarked, errors.New("next element is marked"))
	ErrNotAppend         error = NewError(ErrTNotAppend, errors.New("element cannot be append"))
	ErrNotMarked         error = NewError(ErrTNotMarked, errors.New("elenment cannot be marked"))
	ErrCasConflictOnMark error = NewError(ErrTCasConflictOnMark, errors.New("cas conflict(fail mark)"))
	ErrFirstMarked       error = NewError(ErrTFirstMarked, errors.New("first element is marked"))
	ErrNoSafetyOnAdd     error = NewError(ErrTNoSafety, errors.New("element is not safety to append"))
	ErrNoContinous       error = NewError(ErrTNoContinous, errors.New("element is not continus"))
	//ErrNoSafety          error = NewError(ErrTNoSafety, errors.New("element is not safety to append"))
)

type ListHeadError struct {
	Type uint16
	Info string
	error
}

type OptNewError func(e *ListHeadError)

func NewError(t uint16, err error, opts ...OptNewError) *ListHeadError {

	e := &ListHeadError{Type: t, error: err}

	for _, opt := range opts {
		opt(e)
	}
	return e
}

func Error(oe error, opts ...OptNewError) error {
	e, success := oe.(*ListHeadError)
	if !success {
		return oe
	}

	for _, opt := range opts {
		opt(e)
	}
	return e
}

func ErrorInfo(s string) OptNewError {

	return func(e *ListHeadError) {
		e.Info = s
	}
}

type ListHead struct {
	prev unsafe.Pointer
	next unsafe.Pointer
}

func (head *ListHead) Ptr() unsafe.Pointer {
	return unsafe.Pointer(head)
}

type ListHeadWithError struct {
	head *ListHead
	err  error
}

func (le ListHeadWithError) Error() string {
	return le.err.Error()
}
func (le ListHeadWithError) List() *ListHead {
	return le.head
}

func ListWithError(head *ListHead, err error) ListHeadWithError {
	return ListHeadWithError{head: head, err: err}
}

func GetConcurrentMode() bool {
	return MODE_CONCURRENT
}

func NewEmpty() *ListHead {
	empty := &ListHead{}
	empty.prev = unsafe.Pointer(uintptr(0))
	empty.next = unsafe.Pointer(uintptr(0))
	return empty
}

// head.prev/next = thead
// head.prev = head.diffPtrToHead(thead)
func (head *ListHead) diffPtrToHead(thead *ListHead) unsafe.Pointer {

	t := unsafe.Pointer(thead)
	return head.diffPtrTo(t)

}

func (head *ListHead) diffPtrTo(t unsafe.Pointer) unsafe.Pointer {
	p := unsafe.Pointer(head)

	return unsafe.Add(t, -int(uintptr(p)))

}

func (head *ListHead) Init() {
	// if !MODE_CONCURRENT {
	// 	head.prev = unsafe.Pointer(uintptr(0))
	// 	head.next = unsafe.Pointer(uintptr(0))
	// 	return
	// }

	start := NewEmpty()
	end := NewEmpty()
	head.prev = head.diffPtrToHead(start)
	head.next = head.diffPtrToHead(end)

	start.next = start.diffPtrToHead(head)
	end.prev = end.diffPtrToHead(head)

}

func InitAsEmpty(head *ListHead, tail *ListHead) {

	head.prev = unsafe.Pointer(uintptr(0))
	head.next = unsafe.Pointer(uintptr(0))

	tail.next = unsafe.Pointer(uintptr(0))
	tail.prev = unsafe.Pointer(uintptr(0))

	head.next = head.diffPtrToHead(tail)
	tail.prev = tail.diffPtrToHead(head)

}

func (head *ListHead) InitAsEmpty() {

	end := NewEmpty()

	head.prev = unsafe.Pointer(uintptr(0))
	head.next = unsafe.Pointer(uintptr(0))

	end.next = unsafe.Pointer(uintptr(0))
	end.prev = unsafe.Pointer(uintptr(0))

	head.next = head.diffPtrToHead(end)
	end.prev = end.diffPtrToHead(head)

}

func (head *ListHead) ptr() unsafe.Pointer {

	return unsafe.Pointer(head)

}

func (head *ListHead) DirectNext() *ListHead {
	return head.directNext()
}

func (head *ListHead) directNext() (next *ListHead) {

	if head.next == unsafe.Pointer(nil) {
		return head
	}
	//i := int(uintptr(head.next))

	// FIXME: enable later?
	// if i > 0 && i > 0x1000000 {
	// 	return head
	// }
	// if i < 0 && i < -(0x1000000) {
	// 	return head
	// }

	return (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.next))))
}

func (head *ListHead) nextWaitNoMark() (next *ListHead) {

	//return (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.next))))
	var err error
	next = head.directNext()
	for retry := 100; retry > 0; retry-- {
		if !next.IsMarked() {
			err = nil
			break
		}
		err = ErrMarked
		next = head.directNext()
	}

	if err != nil {
		sharedModeTraverse.SetError(err)
		return nil
	}

	return next

}

func (head *ListHead) skipMarkNext() (next *ListHead) {

	var err error
	next = head.directNext()
	for retry := 100; retry > 0; retry-- {
		if !next.IsMarked() {
			err = nil
			break
		}
		next = NextNoM(head)
		err = nil
		break
	}

	if err != nil {
		sharedModeTraverse.SetError(err)
		return nil
	}
	return next
}

func (head *ListHead) Next(opts ...list_head.TravOpt) (next *ListHead) {
	if len(opts) > 0 {
		sharedModeTraverse.Option(opts...)
	}

	switch sharedModeTraverse.Type() {
	case list_head.TravDirect:
		return head.directNext()
	case list_head.TravWaitNoMark:
		return head.nextWaitNoMark()
	case list_head.TravSkipMark:
		return head.skipMarkNext()
	}

	return head.directNext()
}

func (head *ListHead) DirectPrev() *ListHead {
	return head.directPrev()
}

func (head *ListHead) directPrev() (next *ListHead) {

	return (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.prev))))

}

func (head *ListHead) prevWaitNoMark() (prev *ListHead) {

	//return (*ListHead)(unsafe.Add(head.ptr(), int(uintptr(head.next))))
	var err error
	prev = head.directPrev()
	for retry := 100; retry > 0; retry-- {
		if !prev.IsMarked() {
			err = nil
			break
		}
		err = ErrMarked
		prev = head.directPrev()
	}

	if err != nil {
		sharedModeTraverse.SetError(err)
		return nil
	}

	return prev

}

func (head *ListHead) skipMarkPrev() (prev *ListHead) {

	var err error
	prev = head.directPrev()
	for retry := 100; retry > 0; retry-- {
		if !prev.IsMarked() {
			err = nil
			break
		}
		prev = PrevNoM(head)
		err = nil
		break
	}

	if err != nil {
		sharedModeTraverse.SetError(err)
		return nil
	}
	return prev
}

func (head *ListHead) Prev(opts ...list_head.TravOpt) (next *ListHead) {
	if len(opts) > 0 {
		sharedModeTraverse.Option(opts...)
	}

	switch sharedModeTraverse.Type() {
	case list_head.TravDirect:
		return head.directPrev()
	case list_head.TravWaitNoMark:
		return head.prevWaitNoMark()
	case list_head.TravSkipMark:
		return head.skipMarkPrev()
	}

	return head.directPrev()

}

func toNode(head *ListHead) *ListHead {
	if head.directPrev() == head {
		return head.Next()
	}
	if head.directNext() == head {
		return head.Prev()
	}
	return head

}

func (head *ListHead) IsSingle() bool {

	if !head.Prev().Empty() {
		return false
	}
	if !head.Next().Empty() {
		return false
	}
	return true

}

func (head *ListHead) Empty() bool {
	return head == head.directNext() || head == head.directPrev()
}

func (head *ListHead) P() string {
	return "not implemented"
}
