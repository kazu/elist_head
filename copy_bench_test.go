package elist_head_test

import (
	"fmt"
	"testing"
)

// BenchmarkCopyOnly includes the representation size but no link repair.
func BenchmarkCopyOnly(b *testing.B) {
	for _, n := range []int{16, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			src, dst := make([]benchEntry, n), make([]benchEntry, n)
			for i := range src {
				src[i].key = uint64(i)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(dst, src)
				benchSum = dst[i%n].key
			}
			benchResult = &dst[0]
		})
	}
}
