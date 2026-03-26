package stats

import (
	"fmt"
	"io"
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
