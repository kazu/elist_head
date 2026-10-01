//go:build stephook

package elist_head

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	list_head "github.com/kazu/loncha/lista_encabezado"
)

// Cover every ordered adjacent-operation pair containing a replacement.
// The second insertion placement differs only when left inserts and right
// replaces; the other pairs use the shared gap in both placements.
func TestAdjacentMutationMatrix(t *testing.T) {
	for _, pair := range []struct{ leftOp, rightOp string }{
		{"insert", "replace"}, {"delete", "replace"},
		{"replace", "insert"}, {"replace", "delete"}, {"replace", "replace"},
	} {
		placements := []string{"shared-gap"}
		if pair.leftOp == "insert" && pair.rightOp == "replace" {
			placements = append(placements, "before-each-node")
		}
		for _, placement := range placements {
			for _, releaseFirst := range []string{"left", "right"} {
				t.Run(placement+"/"+pair.leftOp+"_"+pair.rightOp+"/"+releaseFirst+"-released-first", func(t *testing.T) {
					testAdjacentMutationPairAt(t, pair.leftOp, pair.rightOp, releaseFirst, placement, "", "")
				})
			}
		}
	}
}

// Pause after one side has published a partial link or mark, then let an
// operation at the shared gap reach its own hook. These are the points at
// which the other side can observe a transitional neighbor.
func TestAdjacentMutationPartialPublication(t *testing.T) {
	for _, tc := range []struct {
		name, leftOp, rightOp, first, leftAt, rightAt string
	}{
		{"replace_marked_insert", "replace", "insert", "left", "replaceNode.marked", "insert.begin"},
		{"replace_cas2_insert", "replace", "insert", "left", "replaceNode.cas2", "insert.begin"},
		{"replace_marked_delete", "replace", "delete", "left", "replaceNode.marked", "del.begin"},
		{"replace_cas2_delete", "replace", "delete", "left", "replaceNode.cas2", "del.begin"},
		{"insert_cas2_replace", "replace", "insert", "right", "replaceNode.mark", "add.cas2"},
		{"delete_marked_replace", "replace", "delete", "left", "replaceNode.mark", "del.marked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testAdjacentMutationPairAt(t, tc.leftOp, tc.rightOp, tc.first, "shared-gap", tc.leftAt, tc.rightAt)
		})
	}
}

// The replacement passes its initial neighbor check, then publishes the fresh
// right node after deletion has relinked its predecessor and checked the
// successor. Deletion must finish after replacement returns.
func TestAdjacentDeleteReplaceAfterMarks(t *testing.T) {
	for _, schedule := range []string{"delete-prev-checked-first", "replace-marked-first"} {
		t.Run(schedule, func(t *testing.T) { testAdjacentDeleteReplaceAfterMarks(t, schedule) })
	}
}

func testAdjacentDeleteReplaceAfterMarks(t *testing.T, schedule string) {
	nodes := make([]ListHead, 5)
	head, left, right, tail, freshRight := &nodes[0], &nodes[1], &nodes[2], &nodes[3], &nodes[4]
	InitAsEmpty(head, tail)
	for _, n := range []*ListHead{left, right} {
		if _, err := tail.InsertBefore(n); err != nil {
			t.Fatal(err)
		}
	}
	type gate struct {
		ready, release chan struct{}
		once           sync.Once
	}
	newGate := func() *gate { return &gate{ready: make(chan struct{}), release: make(chan struct{})} }
	delBegin, delMarked, delNextMarked, delPrevChecked := newGate(), newGate(), newGate(), newGate()
	replaceMark, replaceMarked := newGate(), newGate()
	if schedule == "delete-prev-checked-first" {
		close(replaceMarked.release)
	} else {
		close(replaceMark.release)
		close(delMarked.release)
		close(delPrevChecked.release)
	}
	defer func() {
		SetStepHook(nil)
		for _, g := range []*gate{delBegin, delMarked, delNextMarked, delPrevChecked, replaceMark, replaceMarked} {
			select {
			case <-g.release:
			default:
				close(g.release)
			}
		}
	}()
	SetStepHook(func(point string, a, b, c *ListHead) {
		var g *gate
		switch {
		case point == "del.begin" && a == left:
			g = delBegin
		case point == "del.marked" && a == left:
			g = delMarked
		case point == "del.nextMarked" && a == left:
			g = delNextMarked
		case point == "del.prevChecked" && a == left:
			g = delPrevChecked
		case point == "replaceNode.mark" && a == right:
			g = replaceMark
		case point == "replaceNode.marked" && a == right:
			g = replaceMarked
		}
		if g != nil {
			g.once.Do(func() { close(g.ready); <-g.release })
		}
	})
	wait := func(label string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s hook not reached", label)
		}
	}
	finish := func(label string, ch <-chan error) {
		t.Helper()
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not finish", label)
		}
	}
	leftDone, rightDone := make(chan error, 1), make(chan error, 1)
	if schedule == "delete-prev-checked-first" {
		go func() { _, err := left.Delete(); leftDone <- err }()
		wait("delete begin", delBegin.ready)
		go func() { rightDone <- right.ReplaceWith(freshRight) }()
		wait("replace pre-mark", replaceMark.ready)
		close(delBegin.release)
		wait("delete next marked", delNextMarked.ready)
		close(delNextMarked.release)
		wait("delete marked", delMarked.ready)
		close(delMarked.release)
		wait("delete predecessor checked", delPrevChecked.ready)
		close(replaceMark.release)
		finish("replace", rightDone)
		close(delPrevChecked.release)
	} else {
		go func() { rightDone <- right.ReplaceWith(freshRight) }()
		wait("replace pre-mark", replaceMark.ready)
		wait("replace marked", replaceMarked.ready)
		go func() { _, err := left.Delete(); leftDone <- err }()
		wait("delete begin", delBegin.ready)
		close(delBegin.release)
		wait("delete next marked", delNextMarked.ready)
		close(replaceMarked.release)
		finish("replace", rightDone)
		close(delNextMarked.release)
	}
	finish("delete", leftDone)
	if head.DirectNext() != freshRight || freshRight.DirectPrev() != head ||
		freshRight.DirectNext() != tail || tail.DirectPrev() != freshRight || freshRight.IsMarked() {
		t.Fatal("replacement lost or list links inconsistent after deletion")
	}
	runtime.KeepAlive(nodes)
}

func testAdjacentMutationPairAt(t *testing.T, leftOp, rightOp, releaseFirst, placement, leftAt, rightAt string) {
	nodes := make([]ListHead, 8)
	head, left, right, tail := &nodes[0], &nodes[1], &nodes[2], &nodes[3]
	leftNew, rightNew := &nodes[4], &nodes[5]
	InitAsEmpty(head, tail)
	if _, err := tail.InsertBefore(left); err != nil {
		t.Fatal(err)
	}
	if _, err := tail.InsertBefore(right); err != nil {
		t.Fatal(err)
	}

	point := func(op string) string {
		switch op {
		case "insert":
			return "insert.begin"
		case "delete":
			return "del.begin"
		default:
			return "replaceNode.mark"
		}
	}
	if leftAt == "" {
		leftAt = point(leftOp)
	}
	if rightAt == "" {
		rightAt = point(rightOp)
	}
	type gate struct {
		ready, release chan struct{}
		once           sync.Once
	}
	leftGate := gate{ready: make(chan struct{}), release: make(chan struct{})}
	rightGate := gate{ready: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		SetStepHook(nil)
		for _, g := range []*gate{&leftGate, &rightGate} {
			select {
			case <-g.release:
			default:
				close(g.release)
			}
		}
	}()
	SetStepHook(func(at string, a, b, c *ListHead) {
		for _, side := range []struct {
			op, at string
			old    *ListHead
			new    *ListHead
			gate   *gate
		}{{leftOp, leftAt, left, leftNew, &leftGate}, {rightOp, rightAt, right, rightNew, &rightGate}} {
			which := side.old
			if side.op == "insert" {
				which = side.new
			}
			if at == side.at && a == which {
				side.gate.once.Do(func() {
					close(side.gate.ready)
					<-side.gate.release
				})
			}
		}
	})

	run := func(op string, old, fresh *ListHead) error {
		switch op {
		case "insert":
			anchor := right
			if placement == "before-each-node" {
				anchor = old
			}
			_, err := anchor.InsertBefore(fresh)
			return err
		case "delete":
			_, err := old.Delete()
			return err
		case "replace":
			return old.ReplaceWith(fresh)
		default:
			panic(op)
		}
	}
	leftDone, rightDone := make(chan error, 1), make(chan error, 1)
	wait := func(label string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not reach its step hook", label)
		}
	}
	if releaseFirst == "left" {
		go func() { leftDone <- run(leftOp, left, leftNew) }()
		wait("left", leftGate.ready)
		go func() { rightDone <- run(rightOp, right, rightNew) }()
	} else {
		go func() { rightDone <- run(rightOp, right, rightNew) }()
		wait("right", rightGate.ready)
		go func() { leftDone <- run(leftOp, left, leftNew) }()
	}
	wait("right", rightGate.ready)
	wait("left", leftGate.ready)
	holdReplacement := leftOp == "replace" && rightOp == "insert" && rightAt == "insert.begin" &&
		(leftAt == "replaceNode.marked" || leftAt == "replaceNode.cas2")
	if holdReplacement {
		close(rightGate.release)
	} else {
		if releaseFirst == "left" {
			close(leftGate.release)
			runtime.Gosched()
			close(rightGate.release)
		} else {
			close(rightGate.release)
			runtime.Gosched()
			close(leftGate.release)
		}
	}
	finish := func(label string, ch <-chan error) error {
		t.Helper()
		select {
		case err := <-ch:
			return err
		case <-time.After(2 * time.Second):
			if label == "left" {
				if holdReplacement {
					t.Log("right completed before left resumed")
				} else {
					select {
					case err := <-rightDone:
						t.Logf("right returned while left stalled: %v", err)
					default:
						t.Log("right has not returned while left stalled")
					}
				}
			}
			for _, item := range []struct {
				name string
				node *ListHead
			}{{"head", head}, {"left", left}, {"right", right}, {"tail", tail}, {"leftNew", leftNew}, {"rightNew", rightNew}} {
				prevMarked, nextMarked := LinkMarks(item.node)
				t.Logf("stalled %s=%p prev=%p next=%p prevMarked=%t nextMarked=%t", item.name, item.node,
					item.node.DirectPrev(), item.node.DirectNext(), prevMarked, nextMarked)
			}
			t.Fatalf("%s operation did not finish", label)
			return nil
		}
	}
	var leftErr, rightErr error
	if holdReplacement {
		rightErr = finish("right", rightDone)
		if !isMarkedInsertRetryLimit(rightErr) {
			t.Fatalf("insert while replacement is paused: got %v, want 100 marked retries", rightErr)
		}
		close(leftGate.release)
		leftErr = finish("left", leftDone)
		if leftErr != nil {
			t.Fatalf("replacement after paused insert: %v", leftErr)
		}
		if _, err := right.InsertBefore(rightNew); err != nil {
			t.Fatalf("retry same insertion after replacement: %v", err)
		}
		t.Log("insert rejected after 100 marked retries while replacement was paused; same node inserted after replacement")
		rightErr = nil
	} else {
		leftErr = finish("left", leftDone)
		rightErr = finish("right", rightDone)
	}
	if leftErr != nil && rightErr != nil {
		t.Fatalf("both operations rejected: left=%v right=%v", leftErr, rightErr)
	}
	successes := 0
	for _, result := range []struct {
		name, op string
		err      error
	}{{"left", leftOp, leftErr}, {"right", rightOp, rightErr}} {
		if result.err == nil {
			successes++
			continue
		}
		allowed := false
		switch result.op {
		case "insert":
			var linkErr *ListHeadError
			allowed = errors.Is(result.err, ErrMarked) || errors.Is(result.err, ErrMoved) ||
				(errors.As(result.err, &linkErr) && linkErr.Type == ErrTCasConflictOnAdd)
		case "replace":
			allowed = errors.Is(result.err, ErrMarked) || errors.Is(result.err, ErrCasConflictOnMark)
		case "delete":
			allowed = false
		}
		if !allowed {
			t.Fatalf("%s %s returned unexpected error: %v", result.name, result.op, result.err)
		}
	}
	t.Logf("successful operations=%d", successes)

	// A conflict is allowed to reject an operation. The resulting list must
	// contain exactly the effects of the operations that reported success.
	expected := map[*ListHead]bool{left: true, right: true}
	for _, outcome := range []struct {
		op         string
		old, fresh *ListHead
		err        error
	}{{leftOp, left, leftNew, leftErr}, {rightOp, right, rightNew, rightErr}} {
		if outcome.err == nil {
			switch outcome.op {
			case "insert":
				expected[outcome.fresh] = true
			case "delete":
				delete(expected, outcome.old)
			case "replace":
				delete(expected, outcome.old)
				expected[outcome.fresh] = true
			}
		}
	}
	seen := make(map[*ListHead]bool)
	prev := head
	for n := head.DirectNext(); n != tail; n = n.DirectNext() {
		if n == nil || seen[n] || len(seen) > len(nodes) {
			t.Fatalf("nil link or cycle after %s/%s: left=%v right=%v", leftOp, rightOp, leftErr, rightErr)
		}
		if n.IsMarked() || !expected[n] || n.DirectPrev() != prev || prev.DirectNext() != n {
			t.Fatalf("unexpected or asymmetric node %p after %s/%s: left=%v right=%v", n, leftOp, rightOp, leftErr, rightErr)
		}
		seen[n] = true
		prev = n
	}
	if tail.DirectPrev() != prev || len(seen) != len(expected) {
		t.Fatal(fmt.Sprintf("lost node or broken tail: got %d, want %d; left=%v right=%v", len(seen), len(expected), leftErr, rightErr))
	}
	// Check the order prescribed by the insertion sites.
	actual := make([]*ListHead, 0, len(seen))
	for n := head.DirectNext(); n != tail; n = n.DirectNext() {
		actual = append(actual, n)
	}
	want := make([]*ListHead, 0, len(expected))
	if placement == "before-each-node" && expected[leftNew] && leftOp == "insert" {
		want = append(want, leftNew)
	}
	if expected[left] {
		want = append(want, left)
	} else if expected[leftNew] && leftOp == "replace" {
		want = append(want, leftNew)
	}
	if placement == "shared-gap" {
		if expected[leftNew] && leftOp == "insert" {
			want = append(want, leftNew)
		}
		if expected[rightNew] && rightOp == "insert" {
			want = append(want, rightNew)
		}
	} else if expected[rightNew] && rightOp == "insert" {
		want = append(want, rightNew)
	}
	if expected[right] {
		want = append(want, right)
	} else if expected[rightNew] && rightOp == "replace" {
		want = append(want, rightNew)
	}
	for i := range want {
		if i >= len(actual) || actual[i] != want[i] {
			t.Fatalf("wrong order at %d: got %p, want %p; left=%v right=%v", i, actual[i], want[i], leftErr, rightErr)
		}
	}
	runtime.KeepAlive(nodes)
}

func isMarkedInsertRetryLimit(err error) bool {
	var retryErr *list_head.ListHeadError
	return errors.As(err, &retryErr) && retryErr.Type == list_head.ErrTOverRetyry &&
		retryErr.Error() == "reach retry limit err=map[element is marked:100]"
}
