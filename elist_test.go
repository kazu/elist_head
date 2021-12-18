package elist_head_test

import (
	"testing"
	"unsafe"

	"github.com/kazu/elist_head"
	list_head "github.com/kazu/loncha/lista_encabezado"
	"github.com/stretchr/testify/assert"
)

func Test_InserBefore(t *testing.T) {

	list := elist_head.NewEmptyList()

	cur := elist_head.ListHead{}
	list.Tail().InsertBefore(&cur)
	assert.Same(t, &cur, cur.Prev().Next())
	prev := elist_head.ListHead{}

	cur.InsertBefore(&prev)

	p := cur.Prev()

	assert.Samef(t, p, &prev, "p=%p &prev=%p prev=%+v cur=%p cur%+v\n",
		p, prev, prev, cur, cur)

}

func Test_Delete(t *testing.T) {
	list := elist_head.NewEmptyList()
	cur := elist_head.ListHead{}

	list.Tail().InsertBefore(&cur)

	p := cur.Prev()

	cur.Prev().Next().Delete()

	assert.Equal(t, true, p.Empty())
	assert.Equal(t, true, p.Empty())

}

type CopyTest struct {
	elist_head.ListHead
}

func Test_CopySlice(t *testing.T) {

	ebase := make([]elist_head.ListHead, 2)
	elist_head.InitAsEmpty(&ebase[0], &ebase[1])
	e := &elist_head.ListHead{}
	ebase[1].InsertBefore(e)

	list1 := make([]CopyTest, 10)
	//list1[0].Init()
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
	// el := elist_head.ListHead{}
	// el.InitAsEmpty()
	els := make([]elist_head.ListHead, 2)
	elist_head.InitAsEmpty(&els[0], &els[1])
	el := &els[0]

	items := make([]elist_head.ListHead, 10000)

	for i := 0; i < 10000; i++ {
		le := list_head.ListHead{}
		le.Init()
		l.Prev().Next().InsertBefore(&le)
		ee := &items[i]
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
		cur := el
		b.StartTimer()
		for i := 0; i < 10000; i++ {
			cur = cur.Next()
		}
		b.StopTimer()
	})

}

func Test_ReplaceSlice(t *testing.T) {

	list := elist_head.NewEmptyList()

	//	list.Tail().InsertBefore(&cur)
	first := &elist_head.ListHead{}
	last := &elist_head.ListHead{}

	elms := make([]elist_head.ListHead, 10)

	list.Tail().InsertBefore(first)
	for i := range elms {
		list.Tail().InsertBefore(&elms[i])
	}
	list.Tail().InsertBefore(last)

	elms2 := make([]elist_head.ListHead, 10)
	elist_head.InitAsEmpty(&elms2[0], &elms2[9])

	for i := range elms2 {
		if i == 0 || i == 9 {
			continue
		}
		elms2[9].InsertBefore(&elms[i])
	}

	first.ReplaceNext(&elms2[0], &elms2[9], last)

	assert.Same(t, first.Next(), &elms2[0])
	assert.Same(t, last.Prev(), &elms2[9])

}
