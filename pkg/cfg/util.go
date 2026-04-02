package cfg

import (
	"bufio"
	"os"
	"strings"
)

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

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isBase 判断字符是否为有效碱基
func isBase(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
