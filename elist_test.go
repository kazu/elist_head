package elist_head_test

import (
	"testing"

	"github.com/kazu/elist_head"
	list_head "github.com/kazu/loncha/lista_encabezado"
	"github.com/stretchr/testify/assert"
)

func Test_InserBefore(t *testing.T) {

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
