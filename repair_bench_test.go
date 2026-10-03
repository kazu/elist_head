package elist_head_test

import (
	elist "github.com/kazu/elist_head"
	"testing"
	"unsafe"
)

func BenchmarkCopyRepair(b *testing.B) {
	for _, n := range []int{16, 1024} {
		name := "16"
		if n == 1024 {
			name = "1024"
		}
		b.Run(name, func(b *testing.B) {
			src, dst := make([]benchEntry, n), make([]benchEntry, n)
			ends := make([]elist.ListHead, 2)
			elist.InitAsEmpty(&ends[0], &ends[1])
			for i := range src {
				if _, err := ends[1].InsertBefore(&src[i].ListHead); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(dst, src)
				if err := elist.RepaireSliceAfterCopy(unsafe.Pointer(&src[0]), unsafe.Pointer(&src[len(src)-1]), unsafe.Pointer(&dst[0]), int(unsafe.Sizeof(src[0])), int(unsafe.Offsetof(src[0].ListHead))); err != nil {
					b.Fatal(err)
				}
				src, dst = dst, src
			}
			b.StopTimer()
			p := ends[0].DirectNext()
			for i := range src {
				if p != &src[i].ListHead {
					b.Fatal("relocation order", i)
				}
				p = p.DirectNext()
			}
		})
	}
}
