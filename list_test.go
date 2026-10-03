package elist_head_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

type typedEntry struct {
	Name string
	elist.ListHead
	Value int
}

type firstLinkEntry struct {
	elist.ListHead
	Value [3]uint64
}

func TestListIdentityAndTraversal(t *testing.T) {
	entries := make([]typedEntry, 5)
	head, tail := &entries[0].ListHead, &entries[4].ListHead
	elist.InitAsEmpty(head, tail)
	list := elist.NewList[typedEntry](unsafe.Offsetof(typedEntry{}.ListHead))
	if list.Next(&entries[0]) != &entries[4] || list.Prev(&entries[4]) != &entries[0] {
		t.Fatal("empty list")
	}
	for i := 1; i <= 3; i++ {
		entries[i].Value = i
		if _, err := tail.InsertBefore(list.Link(&entries[i])); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	if list.Next(&entries[0]) != &entries[1] || list.Prev(&entries[4]) != &entries[3] {
		t.Fatal("element identity")
	}
	if list.Next(&entries[1]) != &entries[2] || list.Prev(&entries[3]) != &entries[2] {
		t.Fatal("directions")
	}
	if list.DirectNext(&entries[1]) != &entries[2] || list.DirectPrev(&entries[3]) != &entries[2] {
		t.Fatal("direct directions")
	}
	if list.Next(&entries[3]) != &entries[4] || list.Prev(&entries[1]) != &entries[0] {
		t.Fatal("terminator identity")
	}
	list.Next(&entries[1]).Value = 20
	if entries[2].Value != 20 || list.Element(list.Link(&entries[2])) != &entries[2] {
		t.Fatal("copied element")
	}
	if _, err := entries[2].Delete(); err != nil {
		t.Fatal(err)
	}
	if list.Next(&entries[1]) != &entries[3] {
		t.Fatal("raw deletion not visible through typed view")
	}
	if err := list.InsertBefore(&entries[3], &entries[2]); err != nil {
		t.Fatal(err)
	}
	if entries[1].ListHead.Next() != list.Link(&entries[2]) {
		t.Fatal("typed insertion not visible through raw links")
	}
	runtime.KeepAlive(entries)
}

func TestListZeroOffsetAndSharedView(t *testing.T) {
	entries := make([]firstLinkEntry, 3)
	elist.InitAsEmpty(&entries[0].ListHead, &entries[2].ListHead)
	a := elist.NewList[firstLinkEntry](0)
	b := a
	if _, err := entries[2].InsertBefore(&entries[1].ListHead); err != nil {
		t.Fatal(err)
	}
	if a.Next(&entries[0]) != &entries[1] || b.Next(&entries[0]) != a.Next(&entries[0]) || a.Element(&entries[0].ListHead) != &entries[0] {
		t.Fatal("view copy or zero-offset recovery")
	}
	runtime.KeepAlive(entries)
}

func TestSampleEntryDirections(t *testing.T) {
	entries := make([]elist.SampleEntry, 5)
	elist.InitAsEmpty(&entries[0].ListHead, &entries[4].ListHead)
	for i := 1; i <= 3; i++ {
		if _, err := entries[4].ListHead.InsertBefore(&entries[i].ListHead); err != nil {
			t.Fatal(err)
		}
	}
	if entries[2].Next() != &entries[3] || entries[2].Prev() != &entries[1] {
		t.Fatal("sample directions")
	}
	runtime.KeepAlive(entries)
}

func TestListConcurrentReaders(t *testing.T) {
	entries := make([]typedEntry, 18)
	list := elist.NewList[typedEntry](unsafe.Offsetof(typedEntry{}.ListHead))
	elist.InitAsEmpty(&entries[0].ListHead, &entries[17].ListHead)
	for i := 1; i <= 16; i++ {
		if _, err := entries[17].InsertBefore(&entries[i].ListHead); err != nil {
			t.Fatal(err)
		}
	}
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			count := 0
			for p := list.Next(&entries[0]); p != &entries[17]; p = list.Next(p) {
				count++
			}
			if count != 16 {
				t.Errorf("count=%d", count)
			}
		}()
	}
	readers.Wait()
	runtime.KeepAlive(entries)
}

func TestLegacyTerminatorsRemainStable(t *testing.T) {
	ends := elist.NewEmptyList()
	if ends.Head() != ends.Head() || ends.Tail() != ends.Tail() || ends.Head().DirectNext() != ends.Tail() {
		t.Fatal("terminators copied")
	}
}

func ExampleList() {
	entries := make([]typedEntry, 4)
	entries[1].Name, entries[2].Name = "alpha", "beta"
	head, tail := &entries[0].ListHead, &entries[3].ListHead
	elist.InitAsEmpty(head, tail)
	list := elist.NewList[typedEntry](unsafe.Offsetof(typedEntry{}.ListHead))
	for i := 1; i <= 2; i++ {
		if _, err := tail.InsertBefore(list.Link(&entries[i])); err != nil {
			panic(err)
		}
	}
	for p := list.Next(&entries[0]); p != &entries[3]; p = list.Next(p) {
		fmt.Println(p.Name)
	}
	runtime.KeepAlive(entries)
	// Output:
	// alpha
	// beta
}
