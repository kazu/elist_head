package elist_head_test

import (
	"testing"
	"unsafe"

	"github.com/kazu/elist_head"
	list_head "github.com/kazu/loncha/lista_encabezado"
	"github.com/stretchr/testify/assert"
)

func Test_InserBefore(t *testing.T) {

	// var head elist_head.Head

	// head = &elist_head.ListHead{}

	cur := elist_head.ListHead{}
	cur.Init()
	prev := elist_head.ListHead{}
	prev.Init()

	cur.InsertBefore(&prev)

	p := cur.Prev()

	assert.Equal(t, p, &prev)

}

func Test_Delete(t *testing.T) {

	cur := elist_head.ListHead{}
	cur.Init()
	p := cur.Prev()

	cur.Prev().Next().Delete()

	assert.Equal(t, true, p.Empty())
	assert.Equal(t, true, p.Empty())

}

type CopyTest struct {
	elist_head.ListHead
}

func Test_CopySlice(t *testing.T) {

	e := &elist_head.ListHead{}
	e.Init()

	list1 := make([]CopyTest, 10)

	list1[0].Init()
	e.DirectNext().InsertBefore(&list1[0].ListHead)

	list2 := make([]CopyTest, 0, 20)
	list2 = append(list2, list1...)

	assert.Same(t, e.DirectNext(), &list1[0].ListHead)
	// assert.NotEqual(t, unsafe.Pointer(e.DirectNext()),
	// 	unsafe.Pointer(&list2[0].ListHead))
	assert.NotSame(t, e.DirectNext(), &list2[0].ListHead)

	a := (*elist_head.ListHead)(nil)
	rr := unsafe.Sizeof(*a)
	rr = unsafe.Sizeof(*e)
	rr = unsafe.Sizeof(e)

	_ = rr

	elist_head.RepaireSliceAfterCopy(
		unsafe.Pointer(&list1[0].ListHead),
		unsafe.Pointer(&list1[9].ListHead),
		unsafe.Pointer(&list2[0].ListHead),
		int(unsafe.Sizeof(elist_head.ListHead{})),
		0)
	assert.NotSame(t, e.DirectNext(), &list1[0].ListHead)
	assert.Same(t, e.DirectNext(), &list2[0].ListHead)

}

func Benchmark_Next(b *testing.B) {

	l := list_head.ListHead{}
	l.InitAsEmpty()
	el := elist_head.ListHead{}
	el.InitAsEmpty()

	items := make([]elist_head.ListHead, 10000)

	for i := 0; i < 10000; i++ {
		le := list_head.ListHead{}
		le.Init()
		l.Prev().Next().InsertBefore(&le)
		ee := &items[i]
		ee.Init()
		el.Prev().Next().InsertBefore(ee)
	}

	b.ResetTimer()
	b.Run("list_head", func(b *testing.B) {
		cur := &l
		b.StartTimer()
		for i := 0; i < 10000; i++ {
			cur = cur.Next()
		}
		b.StopTimer()
	})
	b.ResetTimer()
	b.Run("elist_head", func(b *testing.B) {
		cur := &el
		b.StartTimer()
		for i := 0; i < 10000; i++ {
			cur = cur.Next()
		}
		b.StopTimer()
	})

}
