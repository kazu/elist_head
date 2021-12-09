# elist_head 

elist_head is fastest embedded doubly linked list like a linux kernel's LIST_HEAD for golang


# feature

basic features is the same with [lista_encabezado].
- alternatives for container/list. container/list allocate per set new element. but elist_head is embedded to struct. not over allocatation.
- [lista_encabezado] is prev/next normal pointer. elist_head is relative pointer version. relative pointer should not be used in golang. elist_head element should be used in slices.
- if [lista_encabezado] 's element store to slice, slice append , element pointer changed . so you must change all efercnce element's prev/next. but elist_head is condition referernce in slice. 


# require 

`golang > 1.17`


# basic usaage

sample is in `sample_entry.go`


```go
type SampleEntry struct {
	Name string
	Age  int
	ListHead
}

var EmptySampleEntry *SampleEntry = nil

const sampleEntryOffset = unsafe.Offsetof(EmptySampleEntry.ListHead)

func SampleEntryFromListHead(head *elist_head.ListHead) *SampleEntry {
	return (*SampleEntry)(ElementOf(unsafe.Pointer(head), sampleEntryOffset))
}

func (s *SampleEntry) Offset() uintptr {
	return sampleEntryOffset
}

func (s *SampleEntry) PtrListHead() *elist_head.ListHead {
	return &(s.ListHead)
}

func (s *SampleEntry) fromListHead(l *elist_head.ListHead) *SampleEntry {
	return SampleEntryFromListHead(l)
}

func (s *SampleEntry) FromListHead(l *elist_head.ListHead) List {
	return s.fromListHead(l)
}

func (s *SampleEntry) Prev() *S*ampleEntry {
	return s.fromListHead(s.ListHead.Prev())
}
func (s *SampleEntry) Next() *S*ampleEntry {
	return s.fromListHead(s.ListHead.Prev())
}

func (s *SampleEntry) InsertBefore(n *SampleEntry) (err error) {
	_, err = s.ListHead.InsertBefore(&n.ListHead)
	return
}


func main() {


    list := elist_head.NewEmptyList()

    elems := make([]SampleEntry, 10)


    elem := &elems[0]
    elem.Name = "namae dayo"
    elem.age = 10

    elems[1].Name = "namae dayo"
    elems[1].age = 15
    
    // add elem last of list
    list.Tail().InsertBefore(&elem.ListHead)
    // add elems[1] before elemens[0]
    elem.InsertBefore(&elems[1])

    // get 
    entry := EmptySampleEntry.FromListHead(list.Head().Next()).(*SampleEntry)
    name := entry.Name


    // get before element using Prev()
    nEntry := elem.Prev()

}
```




[lista_encabezado]: https://github.com/kazu/loncha/tree/master/lista_encabezado