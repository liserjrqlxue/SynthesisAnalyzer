package stats

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

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

// readRefFasta 读取FASTA文件，返回第一条序列的序列字符串（忽略标题行）
func readRefFasta(fastaPath string) (string, error) {
	file, err := os.Open(fastaPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var seq strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">") {
			continue
		}
		seq.WriteString(strings.TrimSpace(line))
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return seq.String(), nil
}

// MergeMaps 将 src 中的键值对合并到 dest 中，相同键的值相加。
// 要求 dest 已存在（非 nil），V 为数值类型（支持加法运算）。
func MergeMaps[K comparable, V constraints.Integer | constraints.Float](dest map[K]V, src map[K]V) {
	for k, v := range src {
		dest[k] += v
	}
}
