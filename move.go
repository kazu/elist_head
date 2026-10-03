package elist_head

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// moving is a slice of nodes that a SliceMove replaces in the list with its
// copy. A writer that meets a marked node finds here whether the mark is the
// one of a move, and where the copy of the node is.
type moving struct {
	// src is the memory of the slice; the entry counts while it lives
	src weak.Pointer[byte]
	// source keeps a transient replacement alive until it is retired.
	source unsafe.Pointer
	// dst is the first byte of the copy, and keeps the copy alive
	dst         unsafe.Pointer
	start, last uintptr
	// written is set once the links and the data of the copy are written:
	// another move may lead the copy of its slice to this copy
	written atomic.Bool
	// done is set once the nodes outside the slice lead to the copy: an
	// insert or a delete goes on with the copy of its node
	done atomic.Bool
}

// movings holds the entries of the slices that are or were moved and whose
// memory lives. A reader loads the list without a lock; addMoving replaces
// it.
var movings struct {
	mu   sync.Mutex
	list atomic.Pointer[[]*moving]
}

// addMoving registers the slice from start to last, which is copied to
// dHead. The entry is dropped once the memory of the slice is reclaimed.
func addMoving(sHead, dHead unsafe.Pointer, start, last uintptr, retain bool) *moving {
	m := &moving{dst: dHead, start: start, last: last}
	if retain {
		m.src = weak.Make((*byte)(sHead))
	} else {
		m.source = sHead
	}
	movings.mu.Lock()
	defer movings.mu.Unlock()
	var list []*moving
	if old := movings.list.Load(); old != nil {
		list = append(list, *old...)
	}
	list = append(list, m)
	movings.list.Store(&list)
	// Value of the weak pointer of an entry keeps the slice alive during a
	// collection, so the entries are not tested for it here
	if retain {
		runtime.AddCleanup((*byte)(sHead), dropMoving, m.src)
	}
	return m
}

// dropMoving drops the entry of the slice of src.
func dropMoving(src weak.Pointer[byte]) {
	removeMoving(src, nil)
}

func removeMoving(src weak.Pointer[byte], transient *moving) {
	movings.mu.Lock()
	defer movings.mu.Unlock()
	old := movings.list.Load()
	if old == nil {
		return
	}
	var list []*moving
	for _, e := range *old {
		if (transient != nil && e != transient) || (transient == nil && (e.source != nil || e.src != src)) {
			list = append(list, e)
		}
	}
	movings.list.Store(&list)
}

// retargetMovings leads the entries whose copy is the slice at sHead to
// dHead, the copy of that slice, so that a node that was moved more than
// once is found in the last copy, and no entry keeps a copy alive that was
// moved itself.
func retargetMovings(sHead, dHead unsafe.Pointer) {
	movings.mu.Lock()
	defer movings.mu.Unlock()
	old := movings.list.Load()
	if old == nil {
		return
	}
	var list []*moving
	for _, e := range *old {
		if e.dst == sHead {
			n := &moving{src: e.src, dst: dHead, start: e.start, last: e.last}
			n.written.Store(true)
			n.done.Store(true)
			e = n
		}
		list = append(list, e)
	}
	movings.list.Store(&list)
}

// findMoving returns the entry of the slice that node lies in, or nil.
func findMoving(node *ListHead) *moving {
	list := movings.list.Load()
	if list == nil {
		return nil
	}
	return findMovingIn(*list, node)
}

// findMovingIn returns the entry of list of the slice that node lies in, or
// nil.
func findMovingIn(list []*moving, node *ListHead) *moving {
	p := uintptr(unsafe.Pointer(node))
	for _, e := range list {
		if p >= e.start && p < e.last && (e.source != nil || e.src.Value() != nil) {
			return e
		}
	}
	return nil
}

// markOwn marks the link of head that holds off. moved tells that another
// one marked the link before and head lies in a slice that is moved.
func markOwn(head *ListHead, link *uintptr, off uintptr) (marked, moved bool) {
	if Cas(link, off, off|1) {
		return true, false
	}
	if atomic.LoadUintptr(link) != off|1 {
		return false, false
	}
	// a delete of head that runs at the same time marked it, or a move
	return true, findMoving(head) != nil
}

func (m *moving) copyOf(node *ListHead) *ListHead {
	return (*ListHead)(unsafe.Add(m.dst, uintptr(unsafe.Pointer(node))-m.start))
}

// tookLinked reports whether the move took node, a node of its slice, as
// linked in the list: the copy of node has links. It returns after the links
// of the copies are written.
func (m *moving) tookLinked(node *ListHead) bool {
	for !m.written.Load() {
		stepAt("del.waitWritten", node, nil, nil)
		runtime.Gosched()
	}
	return m.copiedLinked(node)
}

// copiedLinked reports whether the move has written the links of the copies
// and took node, a node of its slice, as linked in the list.
func (m *moving) copiedLinked(node *ListHead) bool {
	if !m.written.Load() {
		return false
	}
	c := m.copyOf(node)
	return atomic.LoadUintptr(&c.prev)|atomic.LoadUintptr(&c.next) != 0
}

// IsMoved reports whether node is marked and lies in a slice that a
// SliceMove replaces or replaced with its copy.
func IsMoved(node *ListHead) bool {
	return node.IsMarked() && findMoving(node) != nil
}

// MovedTo returns the copy of node, when node lies in a slice that a
// SliceMove replaces or replaced with its copy, and nil otherwise. It returns
// after the move is done: the list leads to the copies of the nodes that
// were linked.
func MovedTo(node *ListHead) *ListHead {
	m := findMoving(node)
	if m == nil {
		return nil
	}
	for !m.done.Load() {
		stepAt("move.waitDone", node, nil, nil)
		runtime.Gosched()
	}
	return m.copyOf(node)
}

// FindOrigin returns one node for node and all the copies that moves of
// slices made of it or it is made of: the node of the oldest move whose slice
// is still kept, among the moves to the last copy of node that a move wrote,
// or that last copy when there is none. It does not wait for a move, and does
// not go on to a copy that a move has not written yet. Calls for nodes of one
// line of copies that the callers keep return the same node while its slice
// is kept; once the slice is gone, they return the node of the next oldest
// move still kept, or the last copy.
//
//go:nocheckptr
func FindOrigin(node *ListHead) *ListHead {
	list := movings.list.Load()
	stepAt("origin.loaded", node, nil, nil)
	if list == nil {
		return node
	}
	// one table for both ways: a move added between two reads would lead
	// node to a copy that the other read does not lead back from
	origin := lastCopyIn(*list, node)
	for {
		var from *ListHead
		for _, e := range *list {
			if from = e.sourceOf(origin); from != nil {
				break
			}
		}
		if from == nil {
			return origin
		}
		origin = from
	}
}

// EachOfLine calls fn for each node that FindOrigin may return for node or
// for its copies while the callers keep them: the last copy of node that a
// move wrote, and every node of a slice still kept that moves copied to it,
// directly or through other copies.
func EachOfLine(node *ListHead, fn func(*ListHead)) {
	list := movings.list.Load()
	if list == nil {
		fn(node)
		return
	}
	eachSourceIn(*list, lastCopyIn(*list, node), fn)
}

// eachSourceIn calls fn for node and for every node of a slice still kept
// that the moves of list copied to node, directly or through other copies.
func eachSourceIn(list []*moving, node *ListHead, fn func(*ListHead)) {
	fn(node)
	for _, e := range list {
		if from := e.sourceOf(node); from != nil {
			eachSourceIn(list, from, fn)
		}
	}
}

// lastCopyIn returns the last copy of node that the moves of list wrote, or
// node when they did not. A move writes its copies before the list leads to
// them, and the caller does not touch a copy while the move writes it.
func lastCopyIn(list []*moving, node *ListHead) *ListHead {
	for m := findMovingIn(list, node); m != nil && m.written.Load(); m = findMovingIn(list, node) {
		node = m.copyOf(node)
	}
	return node
}

// sourceOf returns the node of the slice of m that m copied to node, when
// node lies in the copy of m and the slice is still kept, and nil otherwise.
//
//go:nocheckptr
func (m *moving) sourceOf(node *ListHead) *ListHead {
	p, d := uintptr(unsafe.Pointer(node)), uintptr(m.dst)
	if p < d || p >= d+(m.last-m.start) {
		return nil
	}
	s := (*byte)(m.source)
	if s == nil {
		s = m.src.Value()
	}
	if s == nil {
		return nil
	}
	return (*ListHead)(unsafe.Add(unsafe.Pointer(s), p-d))
}

// movingBetween reports whether a marked node between head and to, on the
// side of the next of head when forward is set and of its prev otherwise,
// lies in a slice that is moved. A delete does not pass over such a node: it
// is replaced, not deleted.
//
//go:nocheckptr
func movingBetween(head, to *ListHead, forward bool) bool {
	if movings.list.Load() == nil {
		return false
	}
	step := (*ListHead).directPrev
	if forward {
		step = (*ListHead).directNext
	}
	for cur := step(head); cur != to && cur != head && cur.IsMarked(); cur = step(cur) {
		if findMoving(cur) != nil {
			return true
		}
		if step(cur) == cur {
			break
		}
	}
	return false
}

// InitUnmarked clears the links of head as Init does, prev first and then
// next, and reports whether it cleared both. It stops at the first link that
// is marked and returns false: that link and the ones after it stay as they
// are, and a prev cleared before a marked next stays cleared.
func (head *ListHead) InitUnmarked() bool {
	for _, link := range [...]*uintptr{&head.prev, &head.next} {
		for {
			v := atomic.LoadUintptr(link)
			if v&1 != 0 {
				return false
			}
			if Cas(link, v, 0) {
				break
			}
		}
	}
	return true
}

// InitMarked clears the links of head when both are marked, as a delete of
// head leaves them, and reports whether it did. It leaves the links that an
// insert of head wrote meanwhile.
func (head *ListHead) InitMarked() bool {
	pv := atomic.LoadUintptr(&head.prev)
	nv := atomic.LoadUintptr(&head.next)
	if pv&1 == 0 || nv&1 == 0 {
		return false
	}
	return Cas(&head.prev, pv, 0) && Cas(&head.next, nv, 0)
}

// undoLink puts the link back from the node at the offset from to the node at
// the offset to. It keeps a mark that a move put on the link meanwhile.
func undoLink(link *uintptr, from, to uintptr) {
	for {
		v := atomic.LoadUintptr(link)
		if v&^1 != from {
			return
		}
		if Cas(link, v, to|v&1) {
			return
		}
	}
}

// outerLink is a link of a node outside the slice that leads to a node of
// the slice.
type outerLink struct {
	// t is the node outside the slice, src the node of the slice
	t, src *ListHead
	// prevSide tells that t comes before src: the link is t.next
	prevSide bool
}

// SliceMove replaces the nodes of a slice in the list with their copies, as
// a delete of the nodes and an insert of the copies: FreezeSlice marks the
// links of the nodes as a delete does and writes the links of the copies, the
// caller copies the data of the linked nodes, and Relink leads the nodes
// outside the slice to the copies by CAS.
//
// A marked link never changes by the CAS of an insert or a delete, which
// expect a link without mark. An insert or a delete that meets a node of the
// slice finds its position again until Relink is done; the insert of a node
// of the slice itself returns ErrMoved, and the caller inserts MovedTo of the
// node instead.
type SliceMove struct {
	m            *moving
	sHead, dHead unsafe.Pointer
	size         int
	offset       int
	start, last  uintptr
	moved        int
	outers       []outerLink
}

// copyOf returns the copy of the node src of the slice.
func (mv *SliceMove) copyOf(src *ListHead) *ListHead {
	return (*ListHead)(unsafe.Add(mv.dHead, uintptr(unsafe.Pointer(src))-mv.start))
}

// FreezeSlice starts the move of the slice of nodes from sHead to sTail to
// dHead. When it returns, the links of the nodes of the slice do not change
// any more, and the copy of every node holds the links the node has in the
// list, or no link when the node is not linked.
//
//go:nocheckptr
func FreezeSlice(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int) *SliceMove {
	return freezeSlice(sHead, sTail, dHead, size, offset, true)
}

func freezeSlice(sHead, sTail unsafe.Pointer, dHead unsafe.Pointer, size int, offset int, retain bool) *SliceMove {
	mv := &SliceMove{
		sHead:  sHead,
		dHead:  dHead,
		size:   size,
		offset: offset,
		start:  uintptr(sHead),
		last:   uintptr(sTail) + uintptr(size),
		moved:  int(uintptr(dHead)) - int(uintptr(sHead)),
	}
	mv.m = addMoving(sHead, dHead, mv.start, mv.last, retain)

	first := (*ListHead)(unsafe.Add(sHead, offset))
	step := uintptr(size)
	if !retain {
		step = max(step, mv.last-mv.start-step)
	}
	for i := uintptr(0); i < mv.last-mv.start; i += step {
		src := (*ListHead)(unsafe.Add(sHead, i+uintptr(offset)))
		atomic.OrUintptr(&src.next, 1)
		atomic.OrUintptr(&src.prev, 1)
	}
	stepAt("move.marked", first, nil, nil)

	// An insert or a delete next to a node that made its first CAS before
	// the mark ends or undoes it, and a delete that passed over a node of
	// the slice puts its links back. The links are read again until they
	// are the same twice, with nothing to wait for.
	for pass, changed := 0, true; changed || pass < 2; pass++ {
		changed = false
		mv.outers = mv.outers[:0]
		for i := uintptr(0); i < mv.last-mv.start; i += step {
			src := (*ListHead)(unsafe.Add(sHead, i+uintptr(offset)))
			dst := mv.copyOf(src)
			prev, next := dst.prev, dst.next
			for !mv.copyLinks(src, dst) {
				changed = true
				stepAt("move.wait", src, nil, nil)
				runtime.Gosched()
			}
			if dst.prev != prev || dst.next != next {
				changed = true
			}
		}
	}
	return mv
}

func (mv *SliceMove) inSlice(node *ListHead) bool {
	p := uintptr(unsafe.Pointer(node))
	return p >= mv.start && p < mv.last
}

// copyLinks writes the links of the copy of src from the links of src, and
// reports false when an insert or a delete next to src is between its CASes.
//
//go:nocheckptr
func (mv *SliceMove) copyLinks(src, dst *ListHead) bool {
	pv := atomic.LoadUintptr(&src.prev) &^ 1
	nv := atomic.LoadUintptr(&src.next) &^ 1
	p := (*ListHead)(unsafe.Add(unsafe.Pointer(src), int(pv)))
	q := (*ListHead)(unsafe.Add(unsafe.Pointer(src), int(nv)))

	linkedP := p != src && p.directNext() == src
	linkedQ := q != src && q.directPrev() == src
	if linkedP != linkedQ {
		return false
	}
	if !linkedP {
		dst.prev, dst.next = 0, 0
		return true
	}
	// p is between the CASes of its delete: the node before p leads to src
	// already, and the copy would be linked from p only
	if p.IsMarked() && findMoving(p) == nil {
		if x := PrevNoM(p); x != p && x.directNext() == src {
			return false
		}
	}
	// q is between the CASes of its insert after src: the node after q
	// still links back to src
	if r := q.directNext(); r != q && r.directPrev() == src {
		return false
	}
	if !mv.inSlice(p) {
		mv.outers = append(mv.outers, outerLink{t: p, src: src, prevSide: true})
		pv = IncPointer(pv, -mv.moved)
	}
	if !mv.inSlice(q) {
		mv.outers = append(mv.outers, outerLink{t: q, src: src})
		nv = IncPointer(nv, -mv.moved)
	}
	dst.prev, dst.next = pv, nv
	return true
}

// Linked reports whether the node at the index i of the slice is linked in
// the list. The caller copies the data of the linked nodes before Relink.
func (mv *SliceMove) Linked(i int) bool {
	dst := (*ListHead)(unsafe.Add(mv.dHead, i*mv.size+mv.offset))
	return dst.prev != 0 || dst.next != 0
}

// Relink leads the nodes outside the slice to the copies.
//
//go:nocheckptr
func (mv *SliceMove) Relink() {
	mv.m.written.Store(true)
	stepAt("move.written", (*ListHead)(unsafe.Add(mv.sHead, mv.offset)), nil, nil)

	for _, l := range mv.outers {
		t := l.t
		dst := mv.copyOf(l.src)
		for {
			link := &t.prev
			if l.prevSide {
				link = &t.next
			}
			v := atomic.LoadUintptr(link)
			if v&1 != 0 {
				if m := findMoving(t); m != nil {
					// t is moved too: its move leads the copy of src to
					// the copy of t, this one the copy of t to the copy
					// of src
					for !m.written.Load() {
						stepAt("move.waitWritten", l.src, t, nil)
						runtime.Gosched()
					}
					t = m.copyOf(t)
					continue
				}
			}
			old := t.diffPtrToHead(l.src)
			// the mark of a delete of t stays
			if v&^1 == old && Cas(link, v, t.diffPtrToHead(dst)|v&1) {
				break
			}
			// a delete next to the slice led t to the copy already
			if v&^1 == t.diffPtrToHead(dst) {
				break
			}
			// an insert next to the slice holds the link until its
			// second CAS fails on the mark
			stepAt("move.wait", l.src, t, nil)
			runtime.Gosched()
		}
		if l.prevSide {
			stepAt("repair.prevLinked", l.src, t, dst)
		}
	}
	mv.m.done.Store(true)
	if mv.m.source == nil {
		retargetMovings(mv.sHead, mv.m.dst)
	}
}

// ReplaceSliceAfterCopy replaces a consecutive chain copied into another slice.
// The interior relative links must already be copied; only the boundary links
// are repaired. A single detached node is also allowed. It calls retire after
// publishing the copies, then makes the old boundaries marked self-links.
// The caller must serialize source writers, retire their payloads in retire,
// and never use the old nodes for further list operations. Source nodes must
// not be copies tracked by earlier SliceMoves. Readers may hold old pointers.
func ReplaceSliceAfterCopy(sHead, sTail, dHead unsafe.Pointer, size, offset int, retire func()) {
	mv := freezeSlice(sHead, sTail, dHead, size, offset, false)
	mv.Relink()
	retire()
	step := max(uintptr(mv.size), mv.last-mv.start-uintptr(mv.size))
	for i := uintptr(0); i < mv.last-mv.start; i += step {
		node := (*ListHead)(unsafe.Add(mv.sHead, i+uintptr(mv.offset)))
		atomic.StoreUintptr(&node.prev, 1)
		atomic.StoreUintptr(&node.next, 1)
	}
	removeMoving(weak.Pointer[byte]{}, mv.m)
}
