//go:build stephook

package elist_head_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	elist "github.com/kazu/elist_head"
)

// An explorer runs a few goroutines one step at a time. A step runs one
// goroutine from a step point of elist_head to the next one. Where more than
// one goroutine can take the next step, the schedule says which one does;
// explore replays every schedule, depth first.
//
// A goroutine that takes more than maxRun steps in a row while another one
// can go on is made to wait, so that a goroutine spinning on the progress of
// another one does not make the schedules endless.
const (
	maxRun   = 6
	maxSteps = 400
)

// op is one goroutine of a scenario: it steps the nodes in nodes, and do
// performs its operation.
type op struct {
	name  string
	nodes []*elist.ListHead
	do    func() error
}

type stepEvent struct {
	g     int
	point string
	done  bool
	err   error
}

type run struct {
	ops     []op
	owner   map[*elist.ListHead]int
	events  chan stepEvent
	resume  []chan struct{}
	trace   []string
	errs    []error
	choices []choice
}

type choice struct{ picked, of int }

func (r *run) hook(point string, a, b, c *elist.ListHead) {
	g, ok := r.owner[a]
	if !ok {
		return
	}
	r.events <- stepEvent{g: g, point: point}
	<-r.resume[g]
}

// play runs the ops following the schedule prefix, then picks the first
// goroutine that can go on. It returns false when a goroutine did not reach
// its next step point in time.
func (r *run) play(t *testing.T, prefix []int) bool {
	r.events = make(chan stepEvent)
	r.resume = make([]chan struct{}, len(r.ops))
	r.errs = make([]error, len(r.ops))
	for g := range r.ops {
		r.resume[g] = make(chan struct{})
	}
	elist.SetStepHook(r.hook)
	defer elist.SetStepHook(nil)

	parked := make([]bool, len(r.ops))
	finished := make([]bool, len(r.ops))
	for g := range r.ops {
		go func(g int) {
			r.events <- stepEvent{g: g, point: "start"}
			<-r.resume[g]
			err := r.ops[g].do()
			r.events <- stepEvent{g: g, done: true, err: err}
		}(g)
	}
	wait := func() bool {
		select {
		case ev := <-r.events:
			if ev.done {
				finished[ev.g], r.errs[ev.g] = true, ev.err
				r.trace = append(r.trace, r.ops[ev.g].name+" returns "+fmt.Sprint(ev.err))
			} else {
				parked[ev.g] = true
				r.trace = append(r.trace, r.ops[ev.g].name+" at "+ev.point)
			}
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
	last, inRow := -1, 0
	for steps := 0; ; steps++ {
		var can []int
		for g := range r.ops {
			if parked[g] && !(g == last && inRow >= maxRun) {
				can = append(can, g)
			}
		}
		if len(can) == 0 {
			for g := range r.ops {
				if parked[g] {
					can = append(can, g)
				}
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
		if len(can) > 1 {
			if i := len(r.choices); i < len(prefix) {
				pick = prefix[i]
			}
			r.choices = append(r.choices, choice{pick, len(can)})
		}
		g := can[pick]
		if g == last {
			inRow++
		} else {
			last, inRow = g, 1
		}
		parked[g] = false
		r.resume[g] <- struct{}{}
		if !wait() {
			return false
		}
	}
}

// explore replays every schedule of the ops made by setup, and calls check
// after each one with the errors the ops returned.
func explore(t *testing.T, setup func() ([]op, func(errs []error) error)) {
	t.Helper()
	var prefix []int
	runs := 0
	for {
		ops, check := setup()
		r := &run{ops: ops, owner: map[*elist.ListHead]int{}}
		for g, o := range ops {
			for _, n := range o.nodes {
				r.owner[n] = g
			}
		}
		runs++
		if !r.play(t, prefix) {
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
			t.Logf("%d schedules", runs)
			return
		}
		prefix = make([]int, i+1)
		for j := 0; j < i; j++ {
			prefix[j] = r.choices[j].picked
		}
		prefix[i] = r.choices[i].picked + 1
	}
}

// linked returns the names of the nodes from head to tail, walking forward,
// or an error when the walk backward does not see the same nodes, or a node
// in the list is marked.
func linked(names map[*elist.ListHead]string, head, tail *elist.ListHead) ([]string, error) {
	var fwd, bwd []string
	for cur := head.DirectNext(); cur != tail; cur = cur.DirectNext() {
		if len(fwd) > len(names) {
			return nil, fmt.Errorf("forward walk does not end: %q", fwd)
		}
		if cur.IsMarked() {
			return nil, fmt.Errorf("marked node %s is linked forward: %q", names[cur], fwd)
		}
		fwd = append(fwd, names[cur])
	}
	for cur := tail.DirectPrev(); cur != head; cur = cur.DirectPrev() {
		if len(bwd) > len(names) {
			return nil, fmt.Errorf("backward walk does not end: %q", bwd)
		}
		bwd = append([]string{names[cur]}, bwd...)
	}
	if strings.Join(fwd, " ") != strings.Join(bwd, " ") {
		return nil, fmt.Errorf("forward %q, backward %q", fwd, bwd)
	}
	return fwd, nil
}

// newNamedList links nodes named by names, in this order, between a head and
// a tail, and returns them with a map from each node to its name.
func newNamedList(t *testing.T, names ...string) (head, tail *elist.ListHead, nodes map[string]*elist.ListHead, byNode map[*elist.ListHead]string) {
	entries := make([]typedEntry, len(names)+2)
	head, tail = &entries[0].ListHead, &entries[len(entries)-1].ListHead
	elist.InitAsEmpty(head, tail)
	nodes = map[string]*elist.ListHead{}
	byNode = map[*elist.ListHead]string{head: "head", tail: "tail"}
	for i, name := range names {
		n := &entries[i+1].ListHead
		nodes[name], byNode[n] = n, name
	}
	return head, tail, nodes, byNode
}

// wantLinked returns an error unless the list holds the nodes in want.
func wantLinked(byNode map[*elist.ListHead]string, head, tail *elist.ListHead, want ...string) error {
	got, err := linked(byNode, head, tail)
	if err != nil {
		return err
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		return fmt.Errorf("list holds %q, want %q", got, want)
	}
	return nil
}

// wantUnlinked returns an error unless no node of the list links to n.
func wantUnlinked(byNode map[*elist.ListHead]string, n *elist.ListHead) error {
	for m := range byNode {
		if m != n && (m.DirectNext() == n || m.DirectPrev() == n) && !m.IsMarked() {
			return fmt.Errorf("%s still links to deleted %s", byNode[m], byNode[n])
		}
	}
	return nil
}

// linkAll links nodes, in this order, before tail.
func linkAll(t *testing.T, tail *elist.ListHead, nodes ...*elist.ListHead) {
	t.Helper()
	for _, n := range nodes {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
}

// checkDeleted returns an error unless the delete of n succeeded, no node
// links to n and n is safe to reuse.
func checkDeleted(byNode map[*elist.ListHead]string, n *elist.ListHead, err error) error {
	if err != nil {
		return fmt.Errorf("delete %s: %v", byNode[n], err)
	}
	if err := wantUnlinked(byNode, n); err != nil {
		return err
	}
	if safe, _ := n.IsSafety(); !safe {
		return fmt.Errorf("deleted %s is not safe to reuse", byNode[n])
	}
	return nil
}

func insertOp(name string, at, n *elist.ListHead) op {
	return op{name: name, nodes: []*elist.ListHead{n}, do: func() error { _, err := at.InsertBefore(n); return err }}
}

func deleteOp(name string, n *elist.ListHead) op {
	return op{name: name, nodes: []*elist.ListHead{n}, do: func() error { return n.MarkForDelete() }}
}

// n is inserted before a while p, the node before a, is deleted.
func TestExploreInsertAfterDeletedNode(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "p", "a", "y", "n")
		p, a, y, n := nodes["p"], nodes["a"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, a, y)
		return []op{insertOp("insert n", a, n), deleteOp("delete p", p)}, func(errs []error) error {
			if err := checkDeleted(byNode, p, errs[1]); err != nil {
				return err
			}
			if errs[0] == nil {
				return wantLinked(byNode, head, tail, "n", "a", "y")
			}
			return wantLinked(byNode, head, tail, "a", "y")
		}
	})
}

// n is inserted before y, after a, while a is deleted.
func TestExploreInsertBehindDeletedNode(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "p", "a", "y", "n")
		p, a, y, n := nodes["p"], nodes["a"], nodes["y"], nodes["n"]
		linkAll(t, tail, p, a, y)
		return []op{insertOp("insert n", y, n), deleteOp("delete a", a)}, func(errs []error) error {
			if err := checkDeleted(byNode, a, errs[1]); err != nil {
				return err
			}
			if errs[0] == nil {
				return wantLinked(byNode, head, tail, "p", "n", "y")
			}
			return wantLinked(byNode, head, tail, "p", "y")
		}
	})
}

// n1 and n2 are inserted before a at the same time.
func TestExploreInsertTwiceBeforeNode(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "p", "a", "n1", "n2")
		p, a, n1, n2 := nodes["p"], nodes["a"], nodes["n1"], nodes["n2"]
		linkAll(t, tail, p, a)
		return []op{insertOp("insert n1", a, n1), insertOp("insert n2", a, n2)}, func(errs []error) error {
			for i, err := range errs {
				if err != nil {
					return fmt.Errorf("insert n%d: %v", i+1, err)
				}
			}
			if err := wantLinked(byNode, head, tail, "p", "n1", "n2", "a"); err == nil {
				return nil
			}
			return wantLinked(byNode, head, tail, "p", "n2", "n1", "a")
		}
	})
}

// Two adjacent nodes a and b are deleted at the same time.
func TestExploreDeleteAdjacentNodes(t *testing.T) {
	explore(t, func() ([]op, func([]error) error) {
		head, tail, nodes, byNode := newNamedList(t, "x", "a", "b", "y")
		x, a, b, y := nodes["x"], nodes["a"], nodes["b"], nodes["y"]
		linkAll(t, tail, x, a, b, y)
		return []op{deleteOp("delete a", a), deleteOp("delete b", b)}, func(errs []error) error {
			if err := wantLinked(byNode, head, tail, "x", "y"); err != nil {
				return err
			}
			if err := checkDeleted(byNode, a, errs[0]); err != nil {
				return err
			}
			return checkDeleted(byNode, b, errs[1])
		}
	})
}
