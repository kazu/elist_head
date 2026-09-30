//go:build stephook

package elist_head_test

import (
	"runtime"
	"testing"
	"unsafe"

	elist "github.com/kazu/elist_head"
)

func TestReplaceNodeAdjacentOperations(t *testing.T) {
	if unsafe.Sizeof(elist.ListHead{}) != 2*unsafe.Sizeof(uintptr(0)) {
		t.Fatal("ListHead layout changed")
	}
	for _, point := range []string{"replaceNode.marked", "replaceNode.cas2"} {
		for _, op := range []string{"insertBefore", "insertAfter", "deleteBefore", "deleteAfter", "replaceBefore", "replaceAfter"} {
			t.Run(point+"/"+op, func(t *testing.T) {
				nodes := make([]elist.ListHead, 8)
				h, p, old, q, tail, fresh, other, moved := &nodes[0], &nodes[1], &nodes[2], &nodes[3], &nodes[4], &nodes[5], &nodes[6], &nodes[7]
				elist.InitAsEmpty(h, tail)
				for _, n := range []*elist.ListHead{p, old, q} {
					if _, err := tail.InsertBefore(n); err != nil {
						t.Fatal(err)
					}
				}
				s := newStepper(t)
				stop := s.stopAt(point, old)
				done, errMove := goDo(func() error { return old.ReplaceWith(fresh) })
				stop.waitReached(t)
				if !old.IsMarked() {
					t.Fatal("source not marked")
				}
				started := make(chan struct{})
				otherDone, errOther := goDo(func() error {
					close(started)
					for {
						var err error
						switch op {
						case "insertBefore":
							_, err = p.DirectNext().InsertBefore(other)
						case "insertAfter":
							_, err = q.InsertBefore(other)
						case "deleteBefore":
							return p.MarkForDelete()
						case "deleteAfter":
							return q.MarkForDelete()
						case "replaceBefore":
							err = p.ReplaceWith(moved)
						case "replaceAfter":
							err = q.ReplaceWith(moved)
						}
						if err == nil {
							return nil
						}
						runtime.Gosched()
					}
				})
				<-started
				runtime.Gosched()
				stop.Release()
				waitClosed(t, done, "replace")
				waitClosed(t, otherDone, "adjacent operation")
				if *errMove != nil || *errOther != nil {
					t.Fatalf("replace=%v other=%v", *errMove, *errOther)
				}
				names := map[*elist.ListHead]string{p: "p", old: "old", q: "q", fresh: "fresh", other: "other", moved: "moved"}
				want := map[string][]string{
					"insertBefore": {"p", "other", "fresh", "q"}, "insertAfter": {"p", "fresh", "other", "q"},
					"deleteBefore": {"fresh", "q"}, "deleteAfter": {"p", "fresh"},
					"replaceBefore": {"moved", "fresh", "q"}, "replaceAfter": {"p", "fresh", "moved"},
				}[op]
				assertLinked(t, names, h, tail, want...)
				runtime.KeepAlive(nodes)
			})
		}
	}
}

func TestReplaceNodeConcurrentMarkedNeighbor(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before", false: "after"}[before], func(t *testing.T) {
			nodes := make([]elist.ListHead, 6)
			h, p, old, q, tail, fresh := &nodes[0], &nodes[1], &nodes[2], &nodes[3], &nodes[4], &nodes[5]
			elist.InitAsEmpty(h, tail)
			for _, n := range []*elist.ListHead{p, old, q} {
				if _, err := tail.InsertBefore(n); err != nil {
					t.Fatal(err)
				}
			}
			s := newStepper(t)
			stop := s.stopAt("replaceNode.cas2", old)
			done, replaceErr := goDo(func() error { return old.ReplaceWith(fresh) })
			stop.waitReached(t)
			neighbor := q
			if before {
				neighbor = p
			}
			deleteStop := s.stopAt("del.marked", neighbor)
			deleted, deleteErr := goDo(func() error { return neighbor.MarkForDelete() })
			deleteStop.waitReached(t)
			stop.Release()
			waitClosed(t, done, "replace while neighbor is marked")
			if !neighbor.IsMarked() {
				t.Error("replacement cleared deletion marks")
			}
			deleteStop.Release()
			waitClosed(t, deleted, "neighbor deletion")
			if *replaceErr != nil || *deleteErr != nil {
				t.Fatalf("replace=%v delete=%v", *replaceErr, *deleteErr)
			}
			names := map[*elist.ListHead]string{p: "p", old: "old", q: "q", fresh: "fresh"}
			want := []string{"p", "fresh"}
			if before {
				want = []string{"fresh", "q"}
			}
			assertLinked(t, names, h, tail, want...)
			runtime.KeepAlive(nodes)
		})
	}
}

func TestReplaceNodeInsertionBeforeMark(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before", false: "after"}[before], func(t *testing.T) {
			nodes := make([]elist.ListHead, 5)
			h, old, tail, fresh, inserted := &nodes[0], &nodes[1], &nodes[2], &nodes[3], &nodes[4]
			elist.InitAsEmpty(h, tail)
			if _, err := tail.InsertBefore(old); err != nil {
				t.Fatal(err)
			}
			s := newStepper(t)
			stop := s.stopAt("replaceNode.mark", old)
			done, replaceErr := goDo(func() error {
				for i := 0; i < 100; i++ {
					if err := old.ReplaceWith(fresh); err == nil {
						return nil
					}
					runtime.Gosched()
				}
				return elist.ErrCasConflictOnMark
			})
			stop.waitReached(t)
			next := tail
			if before {
				next = old
			}
			if _, err := next.InsertBefore(inserted); err != nil {
				t.Fatal(err)
			}
			stop.Release()
			waitClosed(t, done, "replacement retry")
			if *replaceErr != nil {
				t.Fatal(*replaceErr)
			}
			names := map[*elist.ListHead]string{old: "old", fresh: "fresh", inserted: "inserted"}
			want := []string{"fresh", "inserted"}
			if before {
				want = []string{"inserted", "fresh"}
			}
			assertLinked(t, names, h, tail, want...)
			runtime.KeepAlive(nodes)
		})
	}
}
