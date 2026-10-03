# elist_head

elist_head is fastest embedded doubly linked list like a linux kernel's LIST_HEAD for golang

Two usage examples are available below:

- [Handwritten methods](#basic-usaage): define conversion and traversal methods on your entry type.
- [Using List[T]](#using-listt): use the same embedded links without writing those methods.


# feature

basic features is the same with [lista_encabezado].
- alternatives for container/list. container/list allocate per set new element. but elist_head is embedded to struct. not over allocatation.
- [lista_encabezado] is prev/next normal pointer. elist_head is relative pointer version. relative pointer should not be used in golang. elist_head element should be used in slices.
- if [lista_encabezado] 's element store to slice, slice append , element pointer changed . so you must change all efercnce element's prev/next. but elist_head is condition referernce in slice.


# require

This branch uses Go 1.27.1 (see `go.mod`). The original v0.2.8 README required Go > 1.17.


# basic usaage

sample is in `sample_entry.go`

The original handwritten approach is shown below, with corrections for the
current API. Both terminators and data entries are embedded in the same slice.
Keep the slice alive while its relative links are in use.

```go
package main

import (
    "fmt"
    "runtime"
    "unsafe"

    elist_head "github.com/kazu/elist_head"
)

type SampleEntry struct {
    Name string
    Age int
    elist_head.ListHead
}

var EmptySampleEntry *SampleEntry = nil

const sampleEntryOffset = unsafe.Offsetof(SampleEntry{}.ListHead)

func SampleEntryFromListHead(head *elist_head.ListHead) *SampleEntry {
    return (*SampleEntry)(elist_head.ElementOf(unsafe.Pointer(head), sampleEntryOffset))
}

func (s *SampleEntry) Offset() uintptr {
    return sampleEntryOffset
}

func (s *SampleEntry) PtrListHead() *elist_head.ListHead {
    return &s.ListHead
}

func (s *SampleEntry) fromListHead(l *elist_head.ListHead) *SampleEntry {
    return SampleEntryFromListHead(l)
}

func (s *SampleEntry) FromListHead(l *elist_head.ListHead) *SampleEntry {
    return s.fromListHead(l)
}

func (s *SampleEntry) Prev() *SampleEntry {
    return s.fromListHead(s.ListHead.Prev())
}

func (s *SampleEntry) Next() *SampleEntry {
    return s.fromListHead(s.ListHead.Next())
}

func (s *SampleEntry) InsertBefore(n *SampleEntry) error {
    _, err := s.ListHead.InsertBefore(&n.ListHead)
    return err
}

func main() {
    storage := make([]SampleEntry, 12)
    head, tail := &storage[0].ListHead, &storage[11].ListHead
    elist_head.InitAsEmpty(head, tail)
    elems := storage[1:11]

    elem := &elems[0]
    elem.Name = "namae dayo"
    elem.Age = 10
    elems[1].Name = "namae dayo"
    elems[1].Age = 15

    // Add elem last, then add elems[1] before elem.
    if _, err := tail.InsertBefore(&elem.ListHead); err != nil {
        panic(err)
    }
    if err := elem.InsertBefore(&elems[1]); err != nil {
        panic(err)
    }

    entry := EmptySampleEntry.FromListHead(head.Next())
    fmt.Println(entry.Name, entry.Age)
    nEntry := elem.Prev()
    fmt.Println(nEntry.Name, nEntry.Age)

    // Stop at the typed tail; Next does not return nil at the end.
    for p := storage[0].Next(); p != &storage[11]; p = p.Next() {
        fmt.Println(p.Name, p.Age)
    }
    runtime.KeepAlive(storage)
}
```

[lista_encabezado]: https://github.com/kazu/loncha/tree/master/lista_encabezado

## Using List[T]

`List[T]` reduces the handwritten methods above without changing the embedded
`ListHead` or copying entries. It stores only the field offset. It does not store
terminators or element slices, search for an owner, allocate elements, or keep
them alive. Existing raw operations and typed operations use the same links.

This is a separate, runnable example. No `Entry.Next` or conversion method is
needed:

```go
package main

import (
    "fmt"
    "runtime"
    "unsafe"

    elist "github.com/kazu/elist_head"
)

type Entry struct {
    Name string
    elist.ListHead
}

func main() {
    entries := make([]Entry, 4)
    entries[1].Name, entries[2].Name = "alpha", "beta"
    head, tail := &entries[0].ListHead, &entries[3].ListHead
    elist.InitAsEmpty(head, tail)
    links := elist.NewList[Entry](unsafe.Offsetof(Entry{}.ListHead))
    for i := 1; i <= 2; i++ {
        if err := links.InsertBefore(&entries[3], &entries[i]); err != nil {
            panic(err)
        }
    }
    for p := links.Next(&entries[0]); p != &entries[3]; p = links.Next(p) {
        fmt.Println(p.Name)
    }
    runtime.KeepAlive(entries)
}
```

Compared with the handwritten example, `links.Link(entry)` obtains the embedded
link, `links.Element(head)` recovers the entry, `links.Next(entry)` and
`links.Prev(entry)` traverse it, and `links.InsertBefore(at, entry)` inserts it.
The example places both terminators inside the `entries` slice and compares
against the typed tail when traversing; `Next` does not return nil at the end.

### Lifetime and compatibility

Pass the actual `unsafe.Offsetof` of the `ListHead` field in `T`. Type parameters
do not prove that the supplied offset or a pointer belongs to the right object.
`Link`, `Element`, and traversal require non-nil pointers to live objects.
Terminators used by typed traversal must also be embedded in `T`. The caller
compares their addresses; there is no automatic conversion of a terminator to
nil. In an empty list, `Next(head)` returns the typed tail and `Prev(tail)` returns
the typed head.

The caller keeps all entries and terminators alive, synchronizes mutations, and
repairs links after copying. These examples use one allocation for entries and
terminators. The original restrictions on relative links across allocations and
copy repair still apply; adding `List[T]` does not resolve them.

The old `List` interface name is replaced by `List[T]`.
`SampleEntry.FromListHead` now returns `*SampleEntry` directly, without a type
assertion. Raw APIs such as `ListHead`, `ElementOf`, and `RepaireSliceAfterCopy`
remain available. This branch also corrects the direction of `SampleEntry.Next`,
pointer-offset arithmetic, mark-bit decoding, and stable terminator access.

### Performance and verification

On amd64, `List[T]` is 8 bytes (the offset); `ListHead` remains 16 bytes and
individual entries do not grow. A temporary view may be optimized away.
Zero allocations per operation does not mean that storing a view takes no space.

`Next` and `Prev` avoid automatic terminator checks and can be inlined. An offset
from a locally constructed view may propagate as a constant, while a view passed
from elsewhere may retain a runtime offset. In particular, `DirectNext` remains
slower in the latter case; equivalent speed is not promised for every usage.

From Nushell:

```nu
with-env {GOTOOLCHAIN: go1.27.1} {
    go test ./...
    go test -race -run 'TestList|TestSampleEntry|TestLegacyTerminators|ExampleList'
    go test -gcflags=all=-d=checkptr=2 -run 'TestList|TestSampleEntry|TestLegacyTerminators|ExampleList'
    go test -run '^$' -bench '^BenchmarkList$|^BenchmarkListView$' -cpu 1
}
```

`BenchmarkList` compares the same `SampleEntry` slice using handwritten methods
(`Readme`), a captured view (`Typed`), a locally constructed view (`Local`), and
a runtime offset (`RuntimeOffset`). The slower captured case is retained in the
comparison. `BenchmarkListView` reports view, offset, node, and entry sizes and
construction allocations. The parent skiplistmap repository provides
`tools/elist-perf/typed.nu` for alternating measurements and CPU profiles.
