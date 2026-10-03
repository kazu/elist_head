//go:build stephook

package elist_head_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// The scenarios of this file: the slice [a, b, c] is moved to [a', b', c']
// while the owner of b, which is not linked yet, inserts it. The list is
// head, x, a, c, y, tail before.

// mvownHook is the step hook of elist_head. explore tells the goroutine of a
// step by the first node of the step, and here the move and the insert step
// on the same nodes (the move waits on b, the insert waits for the copy of a
// or c). The ops put mvownScene.step in the place of the hook of explore: it
// tells the goroutine by its id and passes the step on with a node that only
// this goroutine has.
//
//go:linkname mvownHook github.com/kazu/elist_head.stepHook
var mvownHook atomic.Pointer[elist.StepHook]

const (
	mvownMove = iota
	mvownInsert
)

// mvownMaxTries ends an insert that does not succeed.
const mvownMaxTries = 100

type mvownScene struct {
	// all is the memory of every node: the nodes outside the slice at the
	// front, the slice at all[10:13] and its copy at all[20:23], so that a
	// link with a wrong offset leads to an element without name
	all      []typedEntry
	src, dst []typedEntry

	head, tail *elist.ListHead
	names      map[*elist.ListHead]string
	order      []*elist.ListHead

	// tokens are the nodes that explore knows the goroutines by
	tokens [2]elist.ListHead
	once   sync.Once
	orig   elist.StepHook
	mu     sync.Mutex
	goids  map[uint64]int
}

// enter is called by the goroutine of the op g before anything else.
func (s *mvownScene) enter(g int) {
	s.mu.Lock()
	s.goids[mvdelGoID()] = g
	s.mu.Unlock()
	s.once.Do(func() {
		s.orig = *mvownHook.Load()
		elist.SetStepHook(s.step)
	})
}

func (s *mvownScene) step(point string, a, b, c *elist.ListHead) {
	s.mu.Lock()
	g, ok := s.goids[mvdelGoID()]
	s.mu.Unlock()
	if !ok {
		return
	}
	s.orig(point, &s.tokens[g], b, c)
}

// mvownNew links head, x, a, c, y, tail. a, b and c are the slice; b is not
// linked.
func mvownNew(t *testing.T) *mvownScene {
	t.Helper()
	s := &mvownScene{all: make([]typedEntry, 32), goids: map[uint64]int{}}
	s.src, s.dst = s.all[10:13], s.all[20:23]
	s.head, s.tail = &s.all[0].ListHead, &s.all[3].ListHead
	elist.InitAsEmpty(s.head, s.tail)
	x, y := &s.all[1].ListHead, &s.all[2].ListHead
	s.names = map[*elist.ListHead]string{s.head: "head", s.tail: "tail", x: "x", y: "y"}
	s.order = []*elist.ListHead{s.head, x, y, s.tail}
	for i, name := range []string{"a", "b", "c"} {
		s.src[i].Name, s.src[i].Value = name, i+1
		s.names[&s.src[i].ListHead] = name
		s.names[&s.dst[i].ListHead] = name + "'"
		s.order = append(s.order, &s.src[i].ListHead)
	}
	for i := range s.dst {
		s.order = append(s.order, &s.dst[i].ListHead)
	}
	for _, n := range []*elist.ListHead{x, &s.src[0].ListHead, &s.src[2].ListHead, y} {
		if _, err := s.tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s *mvownScene) node(name string) *elist.ListHead {
	for n, have := range s.names {
		if have == name {
			return n
		}
	}
	return nil
}

// moveOp moves the slice as the item pool does: FreezeSlice, the copy of the
// data of the linked nodes, Relink.
func (s *mvownScene) moveOp() op {
	return op{name: "move", nodes: []*elist.ListHead{&s.tokens[mvownMove]}, do: func() error {
		s.enter(mvownMove)
		mv := elist.FreezeSlice(
			unsafe.Pointer(&s.src[0]),
			unsafe.Pointer(&s.src[len(s.src)-1]),
			unsafe.Pointer(&s.dst[0]),
			int(unsafe.Sizeof(s.src[0])),
			int(unsafe.Offsetof(s.src[0].ListHead)))
		for i := range s.src {
			if mv.Linked(i) {
				s.dst[i].Name, s.dst[i].Value = s.src[i].Name, s.src[i].Value
			}
		}
		mv.Relink()
		return nil
	}}
}

// insertOp inserts b before the node named at, as add2 does: it takes the
// copy of b when b is moved, finds the position again on every try, and
// tries until b or its copy is linked. With init it clears the links of b
// before, as the item pool and the map do for an item they hand out.
func (s *mvownScene) insertOp(at string, init bool) op {
	right := s.node(at)
	return op{name: "insert b", nodes: []*elist.ListHead{&s.tokens[mvownInsert]}, do: func() error {
		s.enter(mvownInsert)
		e := &s.src[1]
		if init {
			// the item pool hands out b
			s.step("own.get", nil, nil, nil)
			e.InitUnmarked()
			// the map stores b
			s.step("own.init", nil, nil, nil)
			if !e.InitUnmarked() && elist.MovedTo(&e.ListHead) == nil {
				e.Init()
			}
		}
		for try := 0; try < mvownMaxTries; try++ {
			if c := elist.MovedTo(&e.ListHead); c != nil {
				moved := (*typedEntry)(unsafe.Add(unsafe.Pointer(c), -int(unsafe.Offsetof(e.ListHead))))
				moved.Name, moved.Value = e.Name, e.Value
				e = moved
			}
			pos := right
			if c := elist.MovedTo(right); c != nil {
				pos = c
			}
			err := pos.TryInsertBefore(&e.ListHead, func(*elist.ListHead) bool { return true })
			if err == nil {
				return nil
			}
			if err == elist.ErrMoved {
				if elist.MovedTo(&e.ListHead) == nil {
					return fmt.Errorf("try %d: insert of %s returns ErrMoved, MovedTo returns nil", try, s.names[&e.ListHead])
				}
				continue
			}
			if err == elist.ErrMarked {
				// returned before the first step point of the insert
				s.step("own.retry", nil, nil, nil)
				continue
			}
			if le, ok := err.(*elist.ListHeadError); ok && le.Type == elist.ErrTCasConflictOnAdd {
				continue
			}
			return fmt.Errorf("try %d: insert of %s before %s: %w", try, s.names[&e.ListHead], s.names[pos], err)
		}
		return fmt.Errorf("insert of b did not end in %d tries", mvownMaxTries)
	}}
}

// walk returns the names of the nodes between head and tail in the order
// from head to tail, walking forward or backward. A node without name ends
// the walk with "?", a marked node gets "(marked)".
func (s *mvownScene) walk(forward bool) []string {
	var got []string
	cur, end := s.head, s.tail
	if !forward {
		cur, end = s.tail, s.head
	}
	for i := 0; ; i++ {
		if i > len(s.names) {
			got = append(got, "...")
			break
		}
		if forward {
			cur = cur.DirectNext()
		} else {
			cur = cur.DirectPrev()
		}
		if cur == end {
			break
		}
		name, ok := s.names[cur]
		if !ok {
			got = append(got, "?")
			break
		}
		if cur.IsMarked() {
			name += "(marked)"
		}
		got = append(got, name)
	}
	if !forward {
		for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
			got[i], got[j] = got[j], got[i]
		}
	}
	return got
}

// dump returns the links of every node.
func (s *mvownScene) dump() string {
	var b strings.Builder
	link := func(n, to *elist.ListHead, marked bool) string {
		name, ok := s.names[to]
		switch {
		case to == n:
			name = "-"
		case !ok:
			name = "?"
		}
		if marked {
			name += "|1"
		}
		return name
	}
	for _, n := range s.order {
		pm, nm := elist.LinkMarks(n)
		fmt.Fprintf(&b, "  %-4s prev=%-6s next=%s\n", s.names[n], link(n, n.DirectPrev(), pm), link(n, n.DirectNext(), nm))
	}
	return b.String()
}

// check returns an error unless the move and the insert returned no error
// and the list holds the nodes named in want, in this order, in both
// directions, with the data of their sources.
func (s *mvownScene) check(errs []error, want []string) error {
	defer runtime.KeepAlive(s)
	for g, err := range errs {
		if err != nil {
			return fmt.Errorf("op %d returns %v\n%s", g, err, s.dump())
		}
	}
	fwd, bwd := strings.Join(s.walk(true), " "), strings.Join(s.walk(false), " ")
	if w := strings.Join(want, " "); fwd != w || bwd != w {
		return fmt.Errorf("forward %q, backward %q, want %q\n%s", fwd, bwd, w, s.dump())
	}
	for i := range s.dst {
		if s.dst[i].Name != s.src[i].Name || s.dst[i].Value != s.src[i].Value {
			return fmt.Errorf("the copy of %s holds (%q, %d), want (%q, %d)\n%s",
				s.src[i].Name, s.dst[i].Name, s.dst[i].Value, s.src[i].Name, s.src[i].Value, s.dump())
		}
	}
	return nil
}

func mvownExplore(t *testing.T, at string, init bool, want ...string) {
	explore(t, func() ([]op, func([]error) error) {
		s := mvownNew(t)
		return []op{s.moveOp(), s.insertOp(at, init)}, func(errs []error) error {
			return s.check(errs, want)
		}
	})
}

// t1: b is inserted before c, between two nodes of the slice.
func TestMoveOwnT1BeforeC(t *testing.T) {
	mvownExplore(t, "c", false, "x", "a'", "b'", "c'", "y")
}

// t2: b is inserted before a, after a node outside the slice.
func TestMoveOwnT2BeforeA(t *testing.T) {
	mvownExplore(t, "a", false, "x", "b'", "a'", "c'", "y")
}

// t3: b is inserted before y, after the last node of the slice.
func TestMoveOwnT3BeforeY(t *testing.T) {
	mvownExplore(t, "y", false, "x", "a'", "c'", "b'", "y")
}

// t4: b is inserted before tail, between two nodes outside the slice.
func TestMoveOwnT4BeforeTail(t *testing.T) {
	mvownExplore(t, "tail", false, "x", "a'", "c'", "y", "b'")
}

// t5: as t1 to t4, and the owner clears the links of b before the insert.
func TestMoveOwnT5InitBeforeC(t *testing.T) {
	mvownExplore(t, "c", true, "x", "a'", "b'", "c'", "y")
}

func TestMoveOwnT5InitBeforeA(t *testing.T) {
	mvownExplore(t, "a", true, "x", "b'", "a'", "c'", "y")
}

func TestMoveOwnT5InitBeforeY(t *testing.T) {
	mvownExplore(t, "y", true, "x", "a'", "c'", "b'", "y")
}

func TestMoveOwnT5InitBeforeTail(t *testing.T) {
	mvownExplore(t, "tail", true, "x", "a'", "c'", "y", "b'")
}
