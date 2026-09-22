package elist_head_test

import (
	"runtime"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

type benchEntry struct {
	key uint64
	elist.ListHead
}

var benchResult *benchEntry
var benchSum uint64

func BenchmarkMigration(b *testing.B) {
	for _, n := range []int{16, 1024} {
		name := "16"
		if n == 1024 {
			name = "1024"
		}
		b.Run(name, func(b *testing.B) {
			items := make([]benchEntry, n)
			ends := make([]elist.ListHead, 2)
			elist.InitAsEmpty(&ends[0], &ends[1])
			for i := range items {
				items[i].key = uint64(i)
				if _, err := ends[1].InsertBefore(&items[i].ListHead); err != nil {
					b.Fatal(err)
				}
			}
			b.Run("Walk", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					count := 0
					for p := ends[0].DirectNext(); p != &ends[1]; p = p.DirectNext() {
						count++
					}
					if count != n {
						b.Fatal(count)
					}
				}
			})
			b.Run("PayloadWalk", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var sum uint64
					for p := ends[0].DirectNext(); p != &ends[1]; p = p.DirectNext() {
						sum += (*benchEntry)(elist.ElementOf(unsafe.Pointer(p), unsafe.Offsetof(benchEntry{}.ListHead))).key
					}
					benchSum = sum
					if sum != uint64(n*(n-1)/2) {
						b.Fatal(sum)
					}
				}
			})
			b.Run("EarlyStop", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if ends[0].DirectNext() != &items[0].ListHead {
						b.Fatal("first")
					}
				}
			})
			b.Run("Recover", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					benchResult = (*benchEntry)(elist.ElementOf(unsafe.Pointer(&items[i%n].ListHead), unsafe.Offsetof(benchEntry{}.ListHead)))
				}
			})
			b.Run("DeleteInsert", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := items[n-1].Delete(func(h *elist.ListHead) error { h.Init(); return nil }); err != nil {
						b.Fatal(err)
					}
					if _, err := ends[1].InsertBefore(&items[n-1].ListHead); err != nil {
						b.Fatal(err)
					}
				}
			})
			runtime.KeepAlive(items)
			runtime.KeepAlive(ends)
		})
	}
}
