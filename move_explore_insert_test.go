//go:build stephook

package elist_head_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

// mvinsList is the list head, x, a, b, y, tail of one schedule. a and b are
// the nodes of src, which a move replaces with their copies a' and b' in dst.
// The nodes outside the slice, src and dst are three arrays, as the arrays
// of a pool are.
type mvinsList struct {
	outer    []typedEntry
	src, dst []typedEntry

	head, x, y, tail *elist.ListHead
	// idle is never linked: a wait is a TryInsertBefore that idle refuses
	idle  *elist.ListHead
	names map[*elist.ListHead]string
	// order holds the nodes that the list may hold
	order []*elist.ListHead

	// done is set by the move after Relink. No step point lies between the
	// end of the move in Relink and this, so that both change in the same
	// step.
	done   atomic.Bool
	linked []bool
}

// mvinsNew links x, a, b and y between head and tail, and makes a node that
// is not linked for every name in news.
func mvinsNew(t *testing.T, news ...string) *mvinsList {
	t.Helper()
	l := &mvinsList{
		outer:  make([]typedEntry, 5+2*len(news)),
		src:    make([]typedEntry, 2),
		dst:    make([]typedEntry, 4),
		linked: make([]bool, 2),
	}
	l.head, l.x, l.y = &l.outer[0].ListHead, &l.outer[1].ListHead, &l.outer[2].ListHead
	l.tail, l.idle = &l.outer[3].ListHead, &l.outer[4].ListHead
	for i, name := range []string{"a", "b"} {
		l.src[i].Name, l.src[i].Value = name, i+1
	}
	elist.InitAsEmpty(l.head, l.tail)
	for _, n := range []*elist.ListHead{l.x, l.a(), l.b(), l.y} {
		if _, err := l.tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	l.names = map[*elist.ListHead]string{
		l.head: "head", l.x: "x", l.y: "y", l.tail: "tail", l.idle: "idle",
		l.a(): "a", l.b(): "b",
		&l.dst[0].ListHead: "a'", &l.dst[1].ListHead: "b'",
	}
	l.order = []*elist.ListHead{l.head, l.x, l.a(), l.b(), &l.dst[0].ListHead, &l.dst[1].ListHead, l.y, l.tail}
	for i, name := range news {
		l.names[l.newNode(i)], l.names[l.waitNode(i)] = name, "wait of "+name
		l.order = append(l.order, l.newNode(i))
	}
	return l
}

func (l *mvinsList) a() *elist.ListHead { return &l.src[0].ListHead }
func (l *mvinsList) b() *elist.ListHead { return &l.src[1].ListHead }

// newNode returns the node that the insert i links, waitNode the node that
// the insert i passes to its waits.
func (l *mvinsList) newNode(i int) *elist.ListHead  { return &l.outer[5+2*i].ListHead }
func (l *mvinsList) waitNode(i int) *elist.ListHead { return &l.outer[6+2*i].ListHead }

// moveOp moves src to dst as the caller of FreezeSlice does: it copies the
// data of the linked nodes between FreezeSlice and Relink.
func (l *mvinsList) moveOp() op {
	return op{name: "move", nodes: []*elist.ListHead{l.a(), l.b()}, do: func() error {
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
		l.done.Store(true)
		return nil
	}}
}

// wait is a step of the insert i that changes nothing. The explorer takes the
// goroutine of a step from the first node of the step point, and the step
// point in the wait of MovedTo names the moved node, which belongs to the
// move. An insert therefore waits here until the move is done, as MovedTo
// does, and calls MovedTo after that. The trace shows a wait as
// "insert.begin".
func (l *mvinsList) wait(i int) {
	l.idle.TryInsertBefore(l.waitNode(i), func(*elist.ListHead) bool { return false })
}

// copyOf returns MovedTo(n), after the move is done.
func (l *mvinsList) copyOf(i int, n *elist.ListHead) *elist.ListHead {
	for elist.IsMoved(n) && !l.done.Load() {
		l.wait(i)
	}
	return elist.MovedTo(n)
}

// insertOp links the new node i before at, as a caller of TryInsertBefore
// does: it finds its position again and tries again until the node is
// linked. The position is the copy of at when at is moved.
func (l *mvinsList) insertOp(i int, at *elist.ListHead) op {
	n := l.newNode(i)
	name := "insert " + l.names[n]
	return op{name: name, nodes: []*elist.ListHead{n, l.waitNode(i)}, do: func() error {
		for {
			pos := at
			if c := l.copyOf(i, at); c != nil {
				pos = c
			}
			err := pos.TryInsertBefore(n, func(*elist.ListHead) bool { return true })
			if err == nil {
				return nil
			}
			le, ok := err.(*elist.ListHeadError)
			if !ok {
				return err
			}
			switch le.Type {
			case elist.ErrTMoved:
				c := l.copyOf(i, n)
				if c == nil {
					return fmt.Errorf("%s: %v, and MovedTo of the new node is nil", name, err)
				}
				n = c
			case elist.ErrTMarked:
				// returned before a step point when pos is marked
				l.wait(i)
			case elist.ErrTCasConflictOnAdd:
			default:
				return fmt.Errorf("%s before %s: %v", name, l.names[pos], err)
			}
		}
	}}
}

// walk returns the names of the nodes between head and tail, in the order
// from head to tail, walking forward or backward. It stops with "?" at a
// link to no node of the scenario, without following it.
func (l *mvinsList) walk(forward bool) string {
	var got []string
	cur, end := l.head, l.tail
	if !forward {
		cur, end = l.tail, l.head
	}
	for {
		if len(got) > len(l.names) {
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
		name, ok := l.names[cur]
		if !ok {
			got = append(got, "?")
			break
		}
		got = append(got, name)
	}
	if !forward {
		for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
			got[i], got[j] = got[j], got[i]
		}
	}
	return strings.Join(got, " ")
}

// links returns the node that every link of every node leads to, and its
// mark.
func (l *mvinsList) links() string {
	var b strings.Builder
	to := func(n *elist.ListHead, off uintptr) {
		name, ok := l.names[(*elist.ListHead)(unsafe.Add(unsafe.Pointer(n), int(off&^1)))]
		if !ok {
			name = fmt.Sprintf("?(%#x)", off&^1)
		}
		b.WriteString(name)
		if off&1 != 0 {
			b.WriteString(" marked")
		}
	}
	for _, n := range l.order {
		b.WriteString("  " + l.names[n] + ": prev -> ")
		to(n, n.OffetPrev())
		b.WriteString(", next -> ")
		to(n, n.OffetNext())
		b.WriteString("\n")
	}
	return b.String()
}

// check returns an error unless every op ended without error and the list
// holds the nodes in want, in both directions. want names the copies a' and
// b', so that a node of src in the list is an error.
func (l *mvinsList) check(errs []error, want string) error {
	defer runtime.KeepAlive(l)
	if err := l.verify(errs, want); err != nil {
		return fmt.Errorf("%v\n%s", err, l.links())
	}
	return nil
}

func (l *mvinsList) verify(errs []error, want string) error {
	for g, err := range errs {
		if err != nil {
			return fmt.Errorf("op %d returned %v", g, err)
		}
	}
	fwd, bwd := l.walk(true), l.walk(false)
	if fwd != bwd {
		return fmt.Errorf("forward %q, backward %q, want %q", fwd, bwd, want)
	}
	if fwd != want {
		return fmt.Errorf("list holds %q, want %q", fwd, want)
	}
	for _, n := range l.order {
		if n == l.a() || n == l.b() {
			continue
		}
		if prev, next := elist.LinkMarks(n); prev || next {
			return fmt.Errorf("%s in the list is marked: prev %v, next %v", l.names[n], prev, next)
		}
	}
	for i := range l.src {
		if !l.linked[i] {
			return fmt.Errorf("Linked(%d) = false, %s was linked", i, l.src[i].Name)
		}
		if l.dst[i].Name != l.src[i].Name || l.dst[i].Value != l.src[i].Value {
			return fmt.Errorf("copy of %s holds %q, %d", l.src[i].Name, l.dst[i].Name, l.dst[i].Value)
		}
	}
	return nil
}

// n is inserted before a, the first node of the slice, while the slice is
// moved.
func TestMoveExploreInsertBeforeFirstOfSlice(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		l := mvinsNew(t, "n")
		return []op{l.moveOp(), l.insertOp(0, l.a())}, func(errs []error) error {
			return l.check(errs, "x n a' b' y")
		}
	})
}

// n is inserted before b, between the two nodes of the slice, while the
// slice is moved.
func TestMoveExploreInsertInsideSlice(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		l := mvinsNew(t, "n")
		return []op{l.moveOp(), l.insertOp(0, l.b())}, func(errs []error) error {
			return l.check(errs, "x a' n b' y")
		}
	})
}

// n is inserted before y, after the last node of the slice, while the slice
// is moved.
func TestMoveExploreInsertAfterLastOfSlice(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		l := mvinsNew(t, "n")
		return []op{l.moveOp(), l.insertOp(0, l.y)}, func(errs []error) error {
			return l.check(errs, "x a' b' n y")
		}
	})
}

// n is inserted before x, the node before the slice, while the slice is
// moved.
func TestMoveExploreInsertBeforeNodeBeforeSlice(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		l := mvinsNew(t, "n")
		return []op{l.moveOp(), l.insertOp(0, l.x)}, func(errs []error) error {
			return l.check(errs, "n x a' b' y")
		}
	})
}

// state returns what the goroutines of a schedule share: the links of the
// nodes and what the move told the inserts.
func (l *mvinsList) state() string {
	return fmt.Sprintf("%s  done %v, linked %v", l.links(), l.done.Load(), l.linked)
}

// mvinsRun runs the ops of one schedule as the run of explore does. explore
// bounds the steps that one goroutine takes in a row, which does not bound
// two goroutines that try again in turn while the third one waits: with two
// inserts and a move it finds no end. mvinsRun looks after each step at the
// step point where every goroutine stands and at the links of the nodes
// instead. When a schedule played before stood the same way, the schedules
// that go on from there are played already or follow, and the goroutines end
// this one each in turn.
//
// What a goroutine holds besides its step point and the nodes of it is not
// looked at.
type mvinsRun struct {
	list    *mvinsList
	ops     []op
	owner   map[*elist.ListHead]int
	events  chan mvinsEvent
	resume  []chan struct{}
	trace   []string
	errs    []error
	choices []choice
}

type mvinsEvent struct {
	g    int
	at   string
	done bool
	err  error
}

func (r *mvinsRun) hook(point string, a, b, c *elist.ListHead) {
	g, ok := r.owner[a]
	if !ok {
		return
	}
	name := func(n *elist.ListHead) string {
		if n == nil {
			return "-"
		}
		return r.list.names[n]
	}
	r.events <- mvinsEvent{g: g, at: point + " (" + name(a) + ", " + name(b) + ", " + name(c) + ")"}
	<-r.resume[g]
}

// play runs the ops following the schedule prefix, then picks the first
// goroutine that can go on. It returns false when a goroutine did not reach
// its next step point in time, or the schedule found no end.
func (r *mvinsRun) play(t *testing.T, prefix []int, seen map[string]bool) bool {
	r.events = make(chan mvinsEvent)
	r.resume = make([]chan struct{}, len(r.ops))
	r.errs = make([]error, len(r.ops))
	for g := range r.ops {
		r.resume[g] = make(chan struct{})
	}
	elist.SetStepHook(r.hook)
	defer elist.SetStepHook(nil)

	parked := make([]bool, len(r.ops))
	at := make([]string, len(r.ops))
	for g := range r.ops {
		go func(g int) {
			r.events <- mvinsEvent{g: g, at: "start"}
			<-r.resume[g]
			err := r.ops[g].do()
			r.events <- mvinsEvent{g: g, done: true, err: err}
		}(g)
	}
	wait := func() bool {
		select {
		case ev := <-r.events:
			if ev.done {
				r.errs[ev.g], at[ev.g] = ev.err, "returns "+fmt.Sprint(ev.err)
			} else {
				parked[ev.g], at[ev.g] = true, "at "+ev.at
			}
			r.trace = append(r.trace, r.ops[ev.g].name+" "+at[ev.g])
			return true
		case <-time.After(5 * time.Second):
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Errorf("a goroutine did not reach a step point\n%s\n%s", strings.Join(r.trace, "\n"), buf[:n])
			return false
		}
	}
	for range r.ops {
		if !wait() {
			return false
		}
	}
	inTurn, turn := false, 0
	for steps := 0; ; steps++ {
		var can []int
		for g := range r.ops {
			if parked[g] {
				can = append(can, g)
			}
		}
		if len(can) == 0 {
			return true
		}
		if steps > maxSteps {
			t.Errorf("no end after %d steps\n%s", steps, strings.Join(r.trace, "\n"))
			for g := range r.ops {
				if parked[g] {
					elist.SetStepHook(nil)
					close(r.resume[g])
				}
			}
			return false
		}
		pick := 0
		switch {
		case inTurn:
			for i, g := range can {
				if g >= turn {
					pick = i
					break
				}
			}
			turn = can[pick] + 1
		case len(can) > 1:
			if i := len(r.choices); i < len(prefix) {
				pick = prefix[i]
			}
			r.choices = append(r.choices, choice{pick, len(can)})
		}
		g := can[pick]
		parked[g] = false
		r.resume[g] <- struct{}{}
		if !wait() {
			return false
		}
		if inTurn || len(r.choices) < len(prefix) {
			// the schedule played before went this way
			continue
		}
		key := strings.Join(at, "\n") + "\n" + r.list.state()
		if seen[key] {
			inTurn = true
			r.trace = append(r.trace, "(stood this way before: every goroutine in turn from here)")
			continue
		}
		seen[key] = true
	}
}

// mvinsExplore replays the schedules of the ops made by setup, each one
// until the goroutines and the links stand as in a schedule played before,
// and calls check after each one with the errors the ops returned.
func mvinsExplore(t *testing.T, setup func() (*mvinsList, []op, func(errs []error) error)) {
	t.Helper()
	var prefix []int
	seen := map[string]bool{}
	runs := 0
	for {
		l, ops, check := setup()
		r := &mvinsRun{list: l, ops: ops, owner: map[*elist.ListHead]int{}}
		for g, o := range ops {
			for _, n := range o.nodes {
				r.owner[n] = g
			}
		}
		runs++
		if !r.play(t, prefix, seen) {
			return
		}
		if err := check(r.errs); err != nil {
			t.Errorf("schedule %v: %v\n%s", prefix, err, strings.Join(r.trace, "\n"))
			return
		}
		i := len(r.choices) - 1
		for ; i >= 0 && r.choices[i].picked+1 == r.choices[i].of; i-- {
		}
		if i < 0 {
			t.Logf("%d schedules, the goroutines and the links stood in %d ways", runs, len(seen))
			return
		}
		prefix = make([]int, i+1)
		for j := 0; j < i; j++ {
			prefix[j] = r.choices[j].picked
		}
		prefix[i] = r.choices[i].picked + 1
	}
}

// n1 is inserted before a and n2 before y at the same time, while the slice
// is moved.
func TestMoveExploreInsertBeforeAndAfterSlice(t *testing.T) {
	mvinsExplore(t, func() (*mvinsList, []op, func([]error) error) {
		l := mvinsNew(t, "n1", "n2")
		return l, []op{l.moveOp(), l.insertOp(0, l.a()), l.insertOp(1, l.y)}, func(errs []error) error {
			return l.check(errs, "x n1 a' b' n2 y")
		}
	})
}
