package stats

import (
	"fmt"
	"io"

	"golang.org/x/exp/constraints"
)

func write1line(w io.Writer, name string, count, reads, base, total int) {
	fmt.Fprintf(
		w,
		"%s,%d,%.4f%%,%.4f%%,%.4f%%\n",
		name,
		count,
		float64(count)/float64(reads)*100,
		float64(count)/float64(base)*100,
		float64(count)/float64(total)*100,
	)
}

// MergeMaps 将 src 中的键值对合并到 dest 中，相同键的值相加。
// 要求 dest 已存在（非 nil），V 为数值类型（支持加法运算）。
func MergeMaps[K comparable, V constraints.Integer | constraints.Float](dest map[K]V, src map[K]V) {
	for k, v := range src {
		dest[k] += v
	}
}
