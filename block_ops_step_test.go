//go:build stephook

package elist_head_test

import (
	"fmt"
	"runtime"
	"testing"

	elist "github.com/kazu/elist_head"
)

func makeBlock(t *testing.T, names map[*elist.ListHead]string, prefix string, n int) (elist.Block, []*elist.ListHead, []string) {
	t.Helper()
	entries := make([]typedEntry, n)
	nodes := make([]*elist.ListHead, n)
	labels := make([]string, n)
	for i := range nodes {
		nodes[i] = &entries[i].ListHead
		labels[i] = fmt.Sprintf("%s%d", prefix, i)
		names[nodes[i]] = labels[i]
	}
	if n == 1 {
		nodes[0].Init()
	} else {
		elist.InitAsEmpty(nodes[0], nodes[n-1])
		for _, node := range nodes[1 : n-1] {
			if _, err := nodes[n-1].InsertBefore(node); err != nil {
				t.Fatal(err)
			}
		}
	}
	return elist.Block{First: nodes[0], Last: nodes[n-1]}, nodes, labels
}

func assertIndependentBlock(t *testing.T, nodes []*elist.ListHead) {
	t.Helper()
	for i, node := range nodes {
		prev, next := node, node
		if i > 0 {
			prev = nodes[i-1]
		}
		if i+1 < len(nodes) {
			next = nodes[i+1]
		}
		if node.DirectPrev() != prev || node.DirectNext() != next {
			t.Fatalf("node %d has wrong internal or self links", i)
		}
	}
}

func TestBlockBasic(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		for _, position := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("nodes=%d/position=%d", n, position), func(t *testing.T) {
				head, tail, old, _, _, names := newReplaceList(t, "p", "q")
				b, nodes, labels := makeBlock(t, names, "b", n)
				assertIndependentBlock(t, nodes)
				if err := b.Delete(); err != elist.ErrNotMarked {
					t.Fatalf("delete of an independent block: %v", err)
				}
				assertIndependentBlock(t, nodes)
				next := tail
				if position < 2 {
					next = old[position]
				}
				for repeat := 0; repeat < 2; repeat++ {
					if err := b.InsertBefore(next); err != nil {
						t.Fatal(err)
					}
					original := []string{"p", "q"}
					want := append([]string{}, original[:position]...)
					want = append(want, labels...)
					want = append(want, original[position:]...)
					assertLinked(t, names, head, tail, want...)
					if err := b.Delete(); err != nil {
						t.Fatal(err)
					}
					assertLinked(t, names, head, tail, "p", "q")
					assertIndependentBlock(t, nodes)
					stale := new(typedEntry)
					if _, err := b.First.InsertBefore(&stale.ListHead); err == nil {
						t.Fatal("insertion using the deleted first node succeeded")
					}
					if _, err := b.Last.InsertBefore(&stale.ListHead); err == nil {
						t.Fatal("insertion using the deleted last node succeeded")
					}
				}
			})
		}
	}
}

func TestBlockCopiedBoundaryLinks(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("nodes=%d", n), func(t *testing.T) {
			head, tail, _, _, _, names := newReplaceList(t)
			src, sn, sl := makeBlock(t, names, "s", n)
			dst, dn, dl := makeBlock(t, names, "d", n)
			if err := src.InsertBefore(tail); err != nil {
				t.Fatal(err)
			}
			// Payload copying initializes the endpoints as individual nodes,
			// while the relative links of the interior are copied verbatim.
			dst.First.Init()
			dst.Last.Init()
			for i := 1; i+1 < n; i++ {
				*dn[i] = *sn[i]
			}
			dst.InitCopiedFrom(src)
			assertIndependentBlock(t, dn)
			assertLinked(t, names, head, tail, sl...)
			if err := dst.InsertBefore(src.First); err != nil {
				t.Fatal(err)
			}
			if err := src.Delete(); err != nil {
				t.Fatal(err)
			}
			assertLinked(t, names, head, tail, dl...)
			assertIndependentBlock(t, sn)
		})
	}

}

func TestBlockConcurrentInsert(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		for _, point := range []string{"block.insert.cas1", "block.insert.cas2"} {
			t.Run(fmt.Sprintf("nodes=%d/%s", n, point), func(t *testing.T) {
				head, tail, _, _, _, names := newReplaceList(t)
				a, an, al := makeBlock(t, names, "a", n)
				b, bn, bl := makeBlock(t, names, "b", n)
				s := newStepper(t)
				stop := s.stopAt(point, a.First)
				done, errA := goDo(func() error { return a.InsertBefore(tail) })
				stop.waitReached(t)
				errB := b.InsertBefore(tail)
				stop.Release()
				waitClosed(t, done, "first insertion")
				for _, retry := range []struct {
					block elist.Block
					nodes []*elist.ListHead
					err   error
				}{{a, an, *errA}, {b, bn, errB}} {
					if retry.err != nil {
						assertIndependentBlock(t, retry.nodes)
						if err := retry.block.InsertBefore(tail); err != nil {
							t.Fatal(err)
						}
					}
				}
				ab := append(append([]string{}, al...), bl...)
				ba := append(append([]string{}, bl...), al...)
				if err := wantLinked(names, head, tail, ab...); err != nil {
					if err := wantLinked(names, head, tail, ba...); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestBlockInsertWithNeighborDelete(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		for _, side := range []int{0, 1} {
			for _, point := range []string{"block.insert.cas1", "block.insert.cas2"} {
				t.Run(fmt.Sprintf("nodes=%d/side=%d/%s", n, side, point), func(t *testing.T) {
					head, tail, old, _, _, names := newReplaceList(t, "p", "q")
					b, nodes, labels := makeBlock(t, names, "b", n)
					s := newStepper(t)
					stop := s.stopAt(point, b.First)
					done, err := goDo(func() error { return b.InsertBefore(old[1]) })
					stop.waitReached(t)
					deleting := s.stopAt("del.marked", old[side])
					deleted, errDelete := goDo(func() error { return old[side].MarkForDelete() })
					deleting.waitReached(t)
					stop.Release()
					waitClosed(t, done, "block insertion")
					deleting.Release()
					waitClosed(t, deleted, "neighbor deletion")
					if *errDelete != nil {
						t.Fatal(*errDelete)
					}
					if *err != nil {
						assertIndependentBlock(t, nodes)
						next := old[1]
						if side == 1 {
							next = tail
						}
						if err := b.InsertBefore(next); err != nil {
							t.Fatal(err)
						}
					}
					want := append([]string{}, labels...)
					if side == 0 {
						want = append(want, "q")
					} else {
						want = append([]string{"p"}, want...)
					}
					assertLinked(t, names, head, tail, want...)
				})
			}
		}
	}
}

func TestBlockAdjacentDelete(t *testing.T) {
	for _, n := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("nodes=%d", n), func(t *testing.T) {
			head, tail, _, _, _, names := newReplaceList(t)
			a, an, _ := makeBlock(t, names, "a", n)
			b, bn, _ := makeBlock(t, names, "b", n)
			if err := a.InsertBefore(tail); err != nil {
				t.Fatal(err)
			}
			if err := b.InsertBefore(tail); err != nil {
				t.Fatal(err)
			}
			s := newStepper(t)
			left, right := s.stopAt("del.marked", a.First), s.stopAt("del.marked", b.First)
			doneA, errA := goDo(a.Delete)
			left.waitReached(t)
			doneB, errB := goDo(b.Delete)
			right.waitReached(t)
			left.Release()
			right.Release()
			waitClosed(t, doneA, "left block deletion")
			waitClosed(t, doneB, "right block deletion")
			if *errA != nil || *errB != nil {
				t.Fatalf("left=%v right=%v", *errA, *errB)
			}
			assertLinked(t, names, head, tail)
			assertIndependentBlock(t, an)
			assertIndependentBlock(t, bn)
		})
	}
}

func TestBlockAdjacentDeleteStages(t *testing.T) {
	points := []string{"block.delete.forward", "block.delete.compressed", "del.nextMarked", "del.marked", "block.delete.first"}
	for _, leftPoint := range points {
		for _, rightPoint := range points {
			for _, firstSide := range []int{0, 1} {
				t.Run(fmt.Sprintf("left=%s/right=%s/first=%d", leftPoint, rightPoint, firstSide), func(t *testing.T) {
					head, tail, _, _, _, names := newReplaceList(t)
					a, an, _ := makeBlock(t, names, "a", 4)
					b, bn, _ := makeBlock(t, names, "b", 4)
					for _, block := range []elist.Block{a, b} {
						if err := block.InsertBefore(tail); err != nil {
							t.Fatal(err)
						}
					}
					s := newStepper(t)
					stops := []*stepStop{s.stopAt(leftPoint, a.First), s.stopAt(rightPoint, b.First)}
					blocks := []elist.Block{a, b}
					doneFirst, errFirst := goDo(blocks[firstSide].Delete)
					stops[firstSide].waitReached(t)
					doneSecond, errSecond := goDo(blocks[1-firstSide].Delete)
					stops[1-firstSide].waitReached(t)
					stops[firstSide].Release()
					stops[1-firstSide].Release()
					waitClosed(t, doneFirst, "first block deletion")
					waitClosed(t, doneSecond, "second block deletion")
					if *errFirst != nil || *errSecond != nil {
						t.Fatalf("first=%v second=%v", *errFirst, *errSecond)
					}
					assertLinked(t, names, head, tail)
					assertIndependentBlock(t, an)
					assertIndependentBlock(t, bn)
				})
			}
		}
	}
}

func TestBlockAdjacentReplacement(t *testing.T) {
	points := []string{"block.insert.cas1", "block.insert.cas2", "block.delete.forward", "block.delete.compressed", "del.marked", "block.delete.first"}
	for _, leftPoint := range points {
		for _, rightPoint := range points {
			for _, firstSide := range []int{0, 1} {
				t.Run(fmt.Sprintf("left=%s/right=%s/first=%d", leftPoint, rightPoint, firstSide), func(t *testing.T) {
					head, tail, _, _, _, names := newReplaceList(t)
					a, an, _ := makeBlock(t, names, "a", 4)
					b, bn, _ := makeBlock(t, names, "b", 4)
					x, _, xl := makeBlock(t, names, "x", 4)
					y, _, yl := makeBlock(t, names, "y", 4)
					old, copies := []elist.Block{a, b}, []elist.Block{x, y}
					for _, block := range old {
						if err := block.InsertBefore(tail); err != nil {
							t.Fatal(err)
						}
					}
					s := newStepper(t)
					var stops [2]*stepStop
					for i, point := range []string{leftPoint, rightPoint} {
						node := old[i].First
						if point == "block.insert.cas1" || point == "block.insert.cas2" {
							node = copies[i].First
						}
						stops[i] = s.stopAt(point, node)
					}
					replace := func(i int) error {
						for copies[i].InsertBefore(old[i].First) != nil {
							runtime.Gosched()
						}
						return old[i].Delete()
					}
					doneFirst, errFirst := goDo(func() error { return replace(firstSide) })
					stops[firstSide].waitReached(t)
					doneSecond, errSecond := goDo(func() error { return replace(1 - firstSide) })
					stops[1-firstSide].waitReached(t)
					stops[firstSide].Release()
					stops[1-firstSide].Release()
					waitClosed(t, doneFirst, "first block replacement")
					waitClosed(t, doneSecond, "second block replacement")
					if *errFirst != nil || *errSecond != nil {
						t.Fatalf("first=%v second=%v", *errFirst, *errSecond)
					}
					assertLinked(t, names, head, tail, append(xl, yl...)...)
					assertIndependentBlock(t, an)
					assertIndependentBlock(t, bn)
				})
			}
		}
	}
}

func TestBlockDeleteWithNeighborOperation(t *testing.T) {
	for _, n := range []int{2, 4} {
		for _, point := range []string{"block.delete.forward", "block.delete.compressed", "block.delete.first"} {
			for _, side := range []int{0, 1} {
				for _, op := range []string{"insert", "delete", "blockInsert"} {
					t.Run(fmt.Sprintf("nodes=%d/%s/side=%d/%s", n, point, side, op), func(t *testing.T) {
						head, tail, old, _, _, names := newReplaceList(t, "p", "q")
						b, nodes, _ := makeBlock(t, names, "b", n)
						if err := b.InsertBefore(old[1]); err != nil {
							t.Fatal(err)
						}
						x, xn, xl := makeBlock(t, names, "x", 1)
						if op == "blockInsert" {
							x, xn, xl = makeBlock(t, names, "x", 3)
						}
						s := newStepper(t)
						stop := s.stopAt(point, b.First)
						done, err := goDo(b.Delete)
						stop.waitReached(t)
						next := old[1]
						if side == 0 {
							next = b.First
							if point == "block.delete.first" {
								next = b.Last
							}
						}
						otherPoint, target := "add.cas1", x.First
						if op == "delete" {
							otherPoint, target = "del.marked", old[side]
						} else if op == "blockInsert" {
							otherPoint = "block.insert.cas1"
						}
						other := s.stopAt(otherPoint, target)
						otherDone, otherErr := goDo(func() error {
							switch op {
							case "delete":
								return old[side].MarkForDelete()
							case "blockInsert":
								return x.InsertBefore(next)
							default:
								_, e := next.InsertBefore(x.First)
								return e
							}
						})
						other.waitReached(t)
						stop.Release()
						other.Release()
						waitClosed(t, done, "block deletion")
						waitClosed(t, otherDone, "neighbor operation")
						if *err != nil {
							t.Fatal(*err)
						}
						assertIndependentBlock(t, nodes)
						if op == "delete" {
							if *otherErr != nil {
								t.Fatal(*otherErr)
							}
							assertLinked(t, names, head, tail, names[old[1-side]])
							return
						}
						if *otherErr != nil {
							assertLinked(t, names, head, tail, "p", "q")
							assertIndependentBlock(t, xn)
							if op == "blockInsert" {
								if err := x.InsertBefore(old[1]); err != nil {
									t.Fatal(err)
								}
							} else if _, err := old[1].InsertBefore(x.First); err != nil {
								t.Fatal(err)
							}
						}
						want := append([]string{"p"}, xl...)
						want = append(want, "q")
						assertLinked(t, names, head, tail, want...)
					})
				}
			}
		}
	}
}
