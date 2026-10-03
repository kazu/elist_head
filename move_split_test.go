package elist_head

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestMoveRetireSplitDestination(t *testing.T) {
	type entry struct {
		value int
		ListHead
	}
	src, mid, dst := make([]entry, 6), make([]entry, 6), make([]entry, 7)
	ends := new([2]ListHead)
	InitAsEmpty(&ends[0], &ends[1])
	for i := range src {
		src[i].value = i
		if _, err := ends[1].InsertBefore(&src[i].ListHead); err != nil {
			t.Fatal(err)
		}
	}
	move := func(from, to []entry) {
		copy(to, from)
		ReplaceSliceAfterCopy(unsafe.Pointer(&from[0]), unsafe.Pointer(&from[len(from)-1]),
			unsafe.Pointer(&to[0]), int(unsafe.Sizeof(entry{})), int(unsafe.Offsetof(entry{}.ListHead)), func() {})
		for i := 1; i+1 < len(from); i++ {
			if from[i].ListHead != to[i].ListHead {
				t.Fatalf("interior relative links changed at %d", i)
			}
		}
	}
	move(src, mid)
	move(mid[:2], dst[:2])
	move(mid[2:], dst[3:])
	for i := range src {
		j := i
		if i >= 2 {
			j++
		}
		if got := MovedTo(&src[i].ListHead); got != nil {
			t.Errorf("retired entry %d still has a move", i)
		}
		if got := FindOrigin(&dst[j].ListHead); got != &dst[j].ListHead {
			t.Errorf("entry %d inherited an old lineage", i)
		}
		if (i == 0 || i == len(src)-1) && (src[i].DirectNext() != &src[i].ListHead || src[i].DirectPrev() != &src[i].ListHead) {
			t.Errorf("retired entry %d still links to neighbours", i)
		}
	}
	if got := FindOrigin(&dst[2].ListHead); got != &dst[2].ListHead {
		t.Error("new gap inherited the lineage of an existing entry")
	}
	runtime.KeepAlive(src)
	runtime.KeepAlive(mid)
	runtime.KeepAlive(dst)
	runtime.KeepAlive(ends)
}
