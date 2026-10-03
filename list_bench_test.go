package elist_head_test

import (
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

var listBenchEntry *elist.SampleEntry
var listBenchView elist.List[elist.SampleEntry]
var listBenchOffset = unsafe.Offsetof(elist.SampleEntry{}.ListHead)

func readmeNextWithOffset(v *elist.SampleEntry, offset uintptr) *elist.SampleEntry {
	h := (*elist.ListHead)(unsafe.Add(unsafe.Pointer(v), offset)).Next()
	return (*elist.SampleEntry)(elist.ElementOf(unsafe.Pointer(h), offset))
}

func readmeDirectNextWithOffset(v *elist.SampleEntry, offset uintptr) *elist.SampleEntry {
	h := (*elist.ListHead)(unsafe.Add(unsafe.Pointer(v), offset)).DirectNext()
	return (*elist.SampleEntry)(elist.ElementOf(unsafe.Pointer(h), offset))
}

// Both variants use SampleEntry, one allocation including terminators, and
// the same Next method on raw links. Both traversals compare their typed tail.
// No callback enters the hot loop.
func BenchmarkList(b *testing.B) {
	for _, n := range []int{16, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			entries := make([]elist.SampleEntry, n+2)
			head, tail := &entries[0].ListHead, &entries[n+1].ListHead
			elist.InitAsEmpty(head, tail)
			for i := 1; i <= n; i++ {
				entries[i].Age = i
				if _, err := tail.InsertBefore(&entries[i].ListHead); err != nil {
					b.Fatal(err)
				}
			}
			view := elist.NewList[elist.SampleEntry](unsafe.Offsetof(elist.SampleEntry{}.ListHead))
			// Keep the captured-view cases below: a local constructor lets the
			// compiler propagate the field offset, unlike a view passed in a closure.
			b.Run("Local", func(b *testing.B) {
				b.Run("DirectWalk", func(b *testing.B) {
					local := elist.NewList[elist.SampleEntry](unsafe.Offsetof(elist.SampleEntry{}.ListHead))
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := local.DirectNext(&entries[0]); p != &entries[n+1]; p = local.DirectNext(p) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("Walk", func(b *testing.B) {
					local := elist.NewList[elist.SampleEntry](unsafe.Offsetof(elist.SampleEntry{}.ListHead))
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := local.Next(&entries[0]); p != &entries[n+1]; p = local.Next(p) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
			})
			b.Run("RuntimeOffset", func(b *testing.B) {
				b.Run("DirectWalk", func(b *testing.B) {
					offset := listBenchOffset
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := readmeDirectNextWithOffset(&entries[0], offset); p != &entries[n+1]; p = readmeDirectNextWithOffset(p, offset) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("Walk", func(b *testing.B) {
					offset := listBenchOffset
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := readmeNextWithOffset(&entries[0], offset); p != &entries[n+1]; p = readmeNextWithOffset(p, offset) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
			})
			b.Run("Readme", func(b *testing.B) {
				b.Run("DirectWalk", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := elist.SampleEntryFromListHead(head.DirectNext()); p != &entries[n+1]; p = elist.SampleEntryFromListHead(p.ListHead.DirectNext()) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("Walk", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := entries[0].Next(); p != &entries[n+1]; p = p.Next() {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("EarlyStop", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						listBenchEntry = entries[0].Next()
					}
					if listBenchEntry != &entries[1] {
						b.Fatal("first element")
					}
				})
				b.Run("Recover", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						listBenchEntry = elist.SampleEntryFromListHead(&entries[i%n+1].ListHead)
					}
				})
			})
			b.Run("Typed", func(b *testing.B) {
				b.Run("DirectWalk", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := view.DirectNext(&entries[0]); p != &entries[n+1]; p = view.DirectNext(p) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("Walk", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						sum := 0
						for p := view.Next(&entries[0]); p != &entries[n+1]; p = view.Next(p) {
							sum += p.Age
						}
						benchSum = uint64(sum)
						if sum != n*(n+1)/2 {
							b.Fatal(sum)
						}
					}
				})
				b.Run("EarlyStop", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						listBenchEntry = view.Next(&entries[0])
					}
					if listBenchEntry != &entries[1] {
						b.Fatal("first element")
					}
				})
				b.Run("Recover", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						listBenchEntry = view.Element(&entries[i%n+1].ListHead)
					}
				})
			})
			runtime.KeepAlive(entries)
		})
	}
}

func BenchmarkListView(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		listBenchView = elist.NewList[elist.SampleEntry](unsafe.Offsetof(elist.SampleEntry{}.ListHead))
	}
	b.ReportMetric(float64(unsafe.Sizeof(listBenchView)), "B/view")
	b.ReportMetric(float64(unsafe.Sizeof(uintptr(0))), "B/offset")
	b.ReportMetric(float64(unsafe.Sizeof(elist.ListHead{})), "B/head")
	b.ReportMetric(float64(unsafe.Sizeof(elist.SampleEntry{})), "B/entry")
}
