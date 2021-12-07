package elist_head

import (
	"unsafe"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

type Head interface {
	Next(opts ...list_head.TravOpt) Head
	DirectNext() Head
	Prev(opts ...list_head.TravOpt) Head
	DirectPrev() Head
	InsertBefore(new Head, opts ...list_head.TravOpt) (Head, error)
	MarkForDelete(opts ...list_head.TravOpt) (err error)
	Empty() bool
	Ptr() unsafe.Pointer
}
