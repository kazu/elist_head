package elist_head

import "unsafe"

type SampleEntry struct {
	Name string
	Age  int
	ListHead
}

var EmptySampleEntry *SampleEntry = nil

const sampleEntryOffset = unsafe.Offsetof(EmptySampleEntry.ListHead)

func SampleEntryFromListHead(head *ListHead) *SampleEntry {
	return (*SampleEntry)(ElementOf(unsafe.Pointer(head), sampleEntryOffset))
}

func (s *SampleEntry) Offset() uintptr {
	return sampleEntryOffset
}

func (s *SampleEntry) PtrListHead() *ListHead {
	return &(s.ListHead)
}

func (s *SampleEntry) fromListHead(l *ListHead) *SampleEntry {
	return SampleEntryFromListHead(l)
}

func (s *SampleEntry) FromListHead(l *ListHead) List {
	return s.fromListHead(l)
}

func (s *SampleEntry) Prev() *SampleEntry {
	return s.fromListHead(s.ListHead.Prev())
}
func (s *SampleEntry) Next() *SampleEntry {
	return s.fromListHead(s.ListHead.Prev())
}

func (s *SampleEntry) InsertBefore(n *SampleEntry) (err error) {
	_, err = s.ListHead.InsertBefore(&n.ListHead)
	return
}

// type List interface {
// 	Offset() uintptr
// 	PtrListHead() *ListHead
// 	FromListHead(*ListHead) List
//}
