//go:build stephook

package elist_head_test

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// The tests of this file run the move of a slice of nodes (FreezeSlice, the
// copy of the data of the linked nodes, Relink) against a delete, and in one
// test against an insert too, in every schedule of the step points.
//
// The explorer of explore_test.go tells the goroutine of a step point by the
// node given to the point. The move and the delete of a node of the slice
// reach points with the same node, so mvdelExplore sets a hook that tells the
// goroutine by its id and hands the point to the explorer with a node that
// stands for the goroutine.

// mvdelList is the list of a scenario: head, x, the nodes of the slice, y
// and tail are linked in this order, and n is not linked. The nodes of the
// slice live in src, their copies in dst.
type mvdelList struct {
	out                 []typedEntry
	src, dst            []typedEntry
	head, tail, x, y, n *elist.ListHead
	names               map[*elist.ListHead]string
	order               []*elist.ListHead
	// linked holds what Linked of the move told for each node of the slice
	linked []bool
}

// the keys of the nodes, which the insert compares as insertInOrder of the
// map does
const (
	mvdelKeyX    = 10
	mvdelKeyN    = 15
	mvdelKeyA    = 20
	mvdelKeyStep = 5
	mvdelKeyY    = 90
	mvdelKeyTail = 100
)

func mvdelNewList(t *testing.T, sliceLen int) *mvdelList {
	t.Helper()
	l := &mvdelList{
		out:    make([]typedEntry, 5),
		src:    make([]typedEntry, sliceLen),
		dst:    make([]typedEntry, sliceLen),
		linked: make([]bool, sliceLen),
	}
	l.head, l.x, l.y, l.tail, l.n = &l.out[0].ListHead, &l.out[1].ListHead, &l.out[2].ListHead, &l.out[3].ListHead, &l.out[4].ListHead
	l.out[1].Value, l.out[2].Value, l.out[3].Value, l.out[4].Value = mvdelKeyX, mvdelKeyY, mvdelKeyTail, mvdelKeyN
	l.names = map[*elist.ListHead]string{l.head: "head", l.x: "x", l.y: "y", l.tail: "tail", l.n: "n"}
	elist.InitAsEmpty(l.head, l.tail)

	l.order = []*elist.ListHead{l.head, l.x}
	linkAll(t, l.tail, l.x)
	for i := range l.src {
		name := string(rune('a' + i))
		l.src[i].Name, l.src[i].Value = name, mvdelKeyA+i*mvdelKeyStep
		l.names[&l.src[i].ListHead], l.names[&l.dst[i].ListHead] = name, name+"'"
		linkAll(t, l.tail, &l.src[i].ListHead)
		l.order = append(l.order, &l.src[i].ListHead)
	}
	linkAll(t, l.tail, l.y)
	l.order = append(l.order, l.y, l.tail, l.n)
	for i := range l.dst {
		l.order = append(l.order, &l.dst[i].ListHead)
	}
	return l
}

func (l *mvdelList) name(n *elist.ListHead) string {
	if n == nil {
		return "-"
	}
	if name, ok := l.names[n]; ok {
		return name
	}
	return "?"
}

// links returns the links of every node: name[<prev >next], with * after a
// link that is marked and 0 for a link that holds no offset.
func (l *mvdelList) links() string {
	var b strings.Builder
	for _, n := range l.order {
		pm, nm := elist.LinkMarks(n)
		side := func(to *elist.ListHead, marked bool) string {
			s := "0"
			if to != n {
				s = l.name(to)
			}
			if marked {
				s += "*"
			}
			return s
		}
		fmt.Fprintf(&b, " %s[<%s >%s]", l.names[n], side(n.DirectPrev(), pm), side(n.DirectNext(), nm))
	}
	return b.String()
}

func (l *mvdelList) keyOf(n *elist.ListHead) int {
	return (*typedEntry)(unsafe.Add(unsafe.Pointer(n), -int(unsafe.Offsetof(l.out[0].ListHead)))).Value
}

// walk returns the names of the nodes between head and tail in the order
// from head to tail, walking forward from head or backward from tail.
func (l *mvdelList) walk(forward bool) ([]string, error) {
	cur, end, dir := l.head, l.tail, "forward"
	if !forward {
		cur, end, dir = l.tail, l.head, "backward"
	}
	var got []string
	for {
		next := cur.DirectNext()
		if !forward {
			next = cur.DirectPrev()
		}
		if next == end {
			break
		}
		name, ok := l.names[next]
		switch {
		case !ok:
			return got, fmt.Errorf("the walk %s leaves the nodes of the test at the link of %s, after %q", dir, l.names[cur], got)
		case next == cur:
			return got, fmt.Errorf("the walk %s stops at %s, whose link holds no offset, after %q", dir, l.names[cur], got)
		case len(got) > len(l.names):
			return got, fmt.Errorf("the walk %s does not end: %q", dir, got)
		}
		got = append(got, name)
		cur = next
	}
	if !forward {
		for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
			got[i], got[j] = got[j], got[i]
		}
	}
	return got, nil
}

// check returns an error unless no op returned an error, and the list holds
// exactly the nodes named in want, forward and backward, none of them
// marked.
func (l *mvdelList) check(ops []op, errs []error, want ...string) error {
	var bad []string
	for g, err := range errs {
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s returned: %v", ops[g].name, err))
		}
	}
	fwd, errF := l.walk(true)
	bwd, errB := l.walk(false)
	for _, err := range []error{errF, errB} {
		if err != nil {
			bad = append(bad, err.Error())
		}
	}
	if errF == nil && errB == nil {
		f, b, w := strings.Join(fwd, " "), strings.Join(bwd, " "), strings.Join(want, " ")
		switch {
		case f != b:
			bad = append(bad, fmt.Sprintf("forward %q, backward %q, want %q", fwd, bwd, want))
		case f != w:
			bad = append(bad, fmt.Sprintf("list holds %q, want %q", fwd, want))
		}
	}
	if errF == nil {
		for _, n := range l.order {
			linked := n == l.head || n == l.tail
			for _, name := range fwd {
				linked = linked || name == l.names[n]
			}
			if pm, nm := elist.LinkMarks(n); linked && (pm || nm) {
				bad = append(bad, fmt.Sprintf("%s is in the list and marked", l.names[n]))
			}
		}
	}
	runtime.KeepAlive(l.out)
	runtime.KeepAlive(l.src)
	runtime.KeepAlive(l.dst)
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nlinks at the end:%s\nLinked of the move: %v", strings.Join(bad, "; "), l.links(), l.linked)
}

// moveOp moves the slice as the expand of the item pool does.
func (l *mvdelList) moveOp() op {
	return op{name: "move", do: func() error {
		mv := elist.FreezeSlice(
			unsafe.Pointer(&l.src[0]),
			unsafe.Pointer(&l.src[len(l.src)-1]),
			unsafe.Pointer(&l.dst[0]),
			int(unsafe.Sizeof(l.src[0])),
			int(unsafe.Offsetof(l.src[0].ListHead)))
		for i := range l.src {
			if l.linked[i] = mv.Linked(i); l.linked[i] {
				l.dst[i].Name, l.dst[i].Value = l.src[i].Name, l.src[i].Value
			}
		}
		mv.Relink()
		return nil
	}}
}

// deleteOp deletes target, and the copy of target when target is moved. With
// initEach it clears the links of every node whose MarkForDelete did not
// return ErrMoved before it asks for the copy, as purgeItem of the map does;
// without, it clears the links of the last node only.
func (l *mvdelList) deleteOp(target *elist.ListHead, initEach bool) op {
	return op{name: "delete " + l.names[target], do: func() error {
		node := target
		for {
			err := node.MarkForDelete()
			switch {
			case err == nil, err == elist.ErrMoved:
			case err == elist.ErrNotMarked && node != target && node.DirectPrev() == node && node.DirectNext() == node:
				// the copy of a node that was deleted before the move
				// looked at it is not linked
			default:
				return fmt.Errorf("MarkForDelete(%s): %v", l.name(node), err)
			}
			if initEach && err != elist.ErrMoved {
				node.Init()
			}
			c := elist.MovedTo(node)
			if c == nil {
				if err == elist.ErrMoved {
					return fmt.Errorf("MarkForDelete(%s): %v, and MovedTo returns nil", l.name(node), err)
				}
				break
			}
			node = c
		}
		node.Init()
		return nil
	}}
}

// insertOp links n before at, or before the copy of at when at is moved, and
// tries again until it is linked.
func (l *mvdelList) insertOp(at, n *elist.ListHead) op {
	return op{name: "insert " + l.names[n], do: func() error {
		for tries := 0; ; tries++ {
			pos := at
			for c := elist.MovedTo(pos); c != nil; c = elist.MovedTo(pos) {
				pos = c
			}
			err := pos.TryInsertBefore(n, func(prev *elist.ListHead) bool {
				return prev == l.head || l.keyOf(prev) <= l.keyOf(n)
			})
			if err == nil {
				return nil
			}
			if err == elist.ErrMoved {
				c := elist.MovedTo(n)
				if c == nil {
					return fmt.Errorf("TryInsertBefore(%s): %v, and MovedTo returns nil", l.name(n), err)
				}
				n = c
				continue
			}
			le, ok := err.(*elist.ListHeadError)
			if !ok || err != elist.ErrMarked && le.Type != elist.ErrTCasConflictOnAdd {
				return fmt.Errorf("TryInsertBefore(%s) before %s, whose prev is %s: %v", l.name(n), l.name(pos), l.name(pos.DirectPrev()), err)
			}
			if tries > 1000 {
				return fmt.Errorf("TryInsertBefore(%s) before %s fails %d times, at last: %v", l.name(n), l.name(pos), tries, err)
			}
			runtime.Gosched()
		}
	}}
}

// mvdelGoID returns the id of the goroutine that calls it.
func mvdelGoID() uint64 {
	var buf [64]byte
	s := strings.TrimPrefix(string(buf[:runtime.Stack(buf[:], false)]), "goroutine ")
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(s, 10, 64)
	return id
}

// mvdelSwitch hands the step points of the goroutines of the ops to the
// explorer, each with the node that stands for its goroutine, and notes the
// links at each point.
type mvdelSwitch struct {
	r    *run
	l    *mvdelList
	mu   sync.Mutex
	byGo map[uint64]*elist.ListHead
	log  []string
}

func (s *mvdelSwitch) enter(standIn *elist.ListHead) {
	s.mu.Lock()
	s.byGo[mvdelGoID()] = standIn
	s.mu.Unlock()
	elist.SetStepHook(s.hook)
}

func (s *mvdelSwitch) note(line string) {
	s.mu.Lock()
	s.log = append(s.log, line+"  |"+s.l.links())
	s.mu.Unlock()
}

func (s *mvdelSwitch) hook(point string, a, b, c *elist.ListHead) {
	s.mu.Lock()
	standIn, ok := s.byGo[mvdelGoID()]
	s.mu.Unlock()
	if !ok {
		return
	}
	s.note(fmt.Sprintf("%s at %s(%s, %s, %s)", s.r.ops[s.r.owner[standIn]].name, point, s.l.name(a), s.l.name(b), s.l.name(c)))
	s.r.hook(point, standIn, b, c)
}

// text returns the notes, without the middle ones when they are more than
// most and most is not 0.
func (s *mvdelSwitch) text(most int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if most == 0 || len(s.log) <= most {
		return strings.Join(s.log, "\n")
	}
	left := fmt.Sprintf("... %d steps left out ...", len(s.log)-most)
	return strings.Join(s.log[:most-3], "\n") + "\n" + left + "\n" + strings.Join(s.log[len(s.log)-3:], "\n")
}

// mvdelNotRun returns the names of the ops that did not return and took no
// step in the last steps of the schedule. The explorer takes the first op
// that can go on when the schedule does not say which one: of three ops, the
// first two can take turns for ever while they wait for the third.
func mvdelNotRun(r *run, last int) []string {
	var names []string
	for _, o := range r.ops {
		returned, steps := false, 0
		for i, line := range r.trace {
			returned = returned || strings.HasPrefix(line, o.name+" returns ")
			if i >= len(r.trace)-last && strings.HasPrefix(line, o.name+" at ") {
				steps++
			}
		}
		if !returned && steps == 0 {
			names = append(names, o.name)
		}
	}
	return names
}

type mvdelCase struct {
	l     *mvdelList
	ops   []op
	check func(errs []error) error
}

// mvdelReports is the number of failing schedules that are printed; the
// others are counted.
const mvdelReports = 3

// mvdelExplore replays every schedule of the ops made by setup as explore
// does, and calls check after each one. It goes on after a schedule whose
// check fails, and stops at a schedule that does not end. It gives up after
// limit schedules when limit is not 0.
func mvdelExplore(t *testing.T, limit int, setup func() mvdelCase) {
	t.Helper()
	var prefix []int
	runs, failed := 0, 0
	seen := map[string]bool{}
	for {
		c := setup()
		r := &run{ops: c.ops, owner: map[*elist.ListHead]int{}}
		s := &mvdelSwitch{r: r, l: c.l, byGo: map[uint64]*elist.ListHead{}}
		for g := range c.ops {
			standIn, do, name := new(elist.ListHead), c.ops[g].do, c.ops[g].name
			r.owner[standIn] = g
			c.ops[g].do = func() error {
				s.enter(standIn)
				err := do()
				s.note(name + " returns " + fmt.Sprint(err))
				return err
			}
		}
		runs++
		if !r.play(t, prefix) {
			for _, name := range mvdelNotRun(r, 100) {
				t.Errorf("the explorer did not run %q in the last 100 steps: two ops that wait took turns, the code was not given the chance to go on", name)
			}
			t.Errorf("schedule %v (number %d) did not end; links at each step:\n%s\nlinks now:%s", prefix, runs, s.text(60), c.l.links())
			t.Logf("%d schedules before it, %d failed", runs-1, failed)
			return
		}
		if err := c.check(r.errs); err != nil {
			failed++
			if failed <= 10 {
				t.Logf("schedule %v (number %d) failed", prefix, runs)
			}
			kind := strings.SplitN(err.Error(), "\n", 2)[0]
			if !seen[kind] && len(seen) < mvdelReports {
				seen[kind] = true
				t.Errorf("schedule %v (number %d): %v\n%s\nlinks at each step:\n%s", prefix, runs, err, strings.Join(r.trace, "\n"), s.text(0))
			}
		}
		runtime.KeepAlive(c.l)

		i := len(r.choices) - 1
		for ; i >= 0 && r.choices[i].picked+1 == r.choices[i].of; i-- {
		}
		if i < 0 {
			t.Logf("%d schedules, %d failed", runs, failed)
			if failed > 0 {
				t.Errorf("%d of %d schedules failed", failed, runs)
			}
			return
		}
		if limit > 0 && runs >= limit {
			if failed > 0 {
				t.Errorf("%d of %d schedules failed", failed, runs)
			}
			t.Skipf("gave up after %d schedules, %d failed: not every schedule was run", runs, failed)
		}
		next := make([]int, i+1)
		for j := 0; j < i; j++ {
			next[j] = r.choices[j].picked
		}
		next[i] = r.choices[i].picked + 1
		prefix = next
	}
}

// mvdelDelete runs the move of a slice of sliceLen nodes against the delete
// of the node named target, and wants the list to hold want.
func mvdelDelete(t *testing.T, sliceLen int, target string, initEach bool, want ...string) {
	t.Helper()
	mvdelExplore(t, 0, func() mvdelCase {
		l := mvdelNewList(t, sliceLen)
		var node *elist.ListHead
		for n, name := range l.names {
			if name == target {
				node = n
			}
		}
		ops := []op{l.moveOp(), l.deleteOp(node, initEach)}
		return mvdelCase{l: l, ops: ops, check: func(errs []error) error { return l.check(ops, errs, want...) }}
	})
}

// t1: a, the first node of the slice [a, b], is deleted while the slice is
// moved.
func TestMvDelDeleteFirstOfSlice(t *testing.T) {
	mvdelDelete(t, 2, "a", false, "x", "b'", "y")
}

// t2: b, the last node of the slice [a, b], is deleted while the slice is
// moved.
func TestMvDelDeleteLastOfSlice(t *testing.T) {
	mvdelDelete(t, 2, "b", false, "x", "a'", "y")
}

// t3: x, the node before the slice [a, b], is deleted while the slice is
// moved.
func TestMvDelDeleteBeforeSlice(t *testing.T) {
	mvdelDelete(t, 2, "x", false, "a'", "b'", "y")
}

// t4: y, the node after the slice [a, b], is deleted while the slice is
// moved.
func TestMvDelDeleteAfterSlice(t *testing.T) {
	mvdelDelete(t, 2, "y", false, "x", "a'", "b'")
}

// t5: a, the only node of the slice [a], is deleted while the slice is
// moved.
func TestMvDelDeleteOnlyNodeOfSlice(t *testing.T) {
	mvdelDelete(t, 1, "a", false, "x", "y")
}

// t1, t2 and t5 with the delete of purgeItem, which clears the links of a
// node before it asks for its copy.
func TestMvDelDeleteFirstOfSliceInitEach(t *testing.T) {
	mvdelDelete(t, 2, "a", true, "x", "b'", "y")
}

func TestMvDelDeleteLastOfSliceInitEach(t *testing.T) {
	mvdelDelete(t, 2, "b", true, "x", "a'", "y")
}

func TestMvDelDeleteOnlyNodeOfSliceInitEach(t *testing.T) {
	mvdelDelete(t, 1, "a", true, "x", "y")
}

// One schedule of t6, replayed with the stepper, since the schedules of t6
// are too many to run them all. The insert of n before a stops between its
// CASes: x.next is n, a.prev is x. The delete of x marks x, leads head.next
// to n, waits for the insert and stops at the start of its next try. The
// move marks a and b, the insert fails on the mark of a.prev and takes n
// out, the move ends, and the delete goes on. The insert then tries again.
func TestMvDelInsertTakenOutAfterDeleteLedToIt(t *testing.T) {
	l := mvdelNewList(t, 2)
	a := &l.src[0].ListHead
	s := newStepper(t)

	ins := s.stopAt("add.cas2", l.n)
	doneI, errI := goDo(func() error {
		return a.TryInsertBefore(l.n, func(prev *elist.ListHead) bool { return l.keyOf(prev) <= l.keyOf(l.n) })
	})
	ins.waitReached(t)

	del1, del2 := s.stopAt("del.marked", l.x), s.stopAt("del.marked", l.x)
	doneD, errD := goDo(l.deleteOp(l.x, false).do)
	del1.waitReached(t)
	del1.Release()
	del2.waitReached(t)
	t.Logf("the delete led head.next to n:%s", l.links())

	marked := s.stopAt("move.marked", a)
	doneM, _ := goDo(l.moveOp().do)
	marked.waitReached(t)
	marked.Release()
	ins.Release()
	waitClosed(t, doneI, "insert n")
	if *errI == nil {
		t.Fatalf("the insert of n linked n between its CASes:%s", l.links())
	}
	waitClosed(t, doneM, "move")
	t.Logf("the insert took n out (%v) and the move ended:%s", *errI, l.links())

	del2.Release()
	waitClosed(t, doneD, "delete x")
	t.Logf("the delete ended:%s", l.links())

	ops := []op{{name: "delete x"}, l.insertOp(a, l.n)}
	if err := l.check(ops, []error{*errD, ops[1].do()}, "n", "a'", "b'", "y"); err != nil {
		t.Error(err)
	}
}

// t6: x is deleted and n is inserted before a while the slice [a, b] is
// moved. The schedules are more than 1500000, so the test gives up after
// 60000 of them, or after MVDEL_LIMIT of them. The schedules are tried in
// the order of the ops, which MVDEL_ORDER gives with the letters m (move), d
// (delete) and i (insert): the first op takes the first steps in the
// schedules that are tried first. With the move as the last op, the explorer
// lets the delete and the insert wait for the move in turns and never runs
// the move; mvdelExplore tells so when such a schedule does not end.
func TestMvDelDeleteBeforeSliceAndInsert(t *testing.T) {
	limit, order := 60000, "mdi"
	if s := os.Getenv("MVDEL_LIMIT"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("MVDEL_LIMIT: %v", err)
		}
		limit = n
	}
	if s := os.Getenv("MVDEL_ORDER"); s != "" {
		order = s
	}
	mvdelExplore(t, limit, func() mvdelCase {
		l := mvdelNewList(t, 2)
		byLetter := map[rune]op{'m': l.moveOp(), 'd': l.deleteOp(l.x, false), 'i': l.insertOp(&l.src[0].ListHead, l.n)}
		var ops []op
		for _, letter := range order {
			o, ok := byLetter[letter]
			if !ok {
				t.Fatalf("MVDEL_ORDER: %q", order)
			}
			ops = append(ops, o)
			delete(byLetter, letter)
		}
		if len(byLetter) != 0 {
			t.Fatalf("MVDEL_ORDER: %q", order)
		}
		return mvdelCase{l: l, ops: ops, check: func(errs []error) error {
			return l.check(ops, errs, "n", "a'", "b'", "y")
		}}
	})
}
