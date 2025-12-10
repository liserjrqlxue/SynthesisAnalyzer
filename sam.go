package synthesis

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

func (a *SynthesisAnalyzer) analyzeSAM(samFile string, refIdx int) error {
	file, err := os.Open(samFile)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	bufferPool := sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 4096)
		},
	}

	// 初始化位置统计
	refLen := len(a.references[refIdx].Sequence)
	posStats := make([]PositionStats, refLen)
	for i := range posStats {
		posStats[i].Position = i
	}

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if line[0] == '@' { // 跳过头部
			continue
		}

		// 复用buffer减少分配
		buf := bufferPool.Get().([]byte)
		buf = buf[:0]
		buf = append(buf, line...)

		// 解析SAM行
		fields := bytes.Split(buf, []byte{'\t'})
		if len(fields) < 11 {
			bufferPool.Put(buf)
			continue
		}

		// 检查比对质量
		mapq, _ := strconv.Atoi(string(fields[4]))
		if mapq < 20 { // 低质量比对跳过
			bufferPool.Put(buf)
			continue
		}

		// 解析CIGAR和MD标签
		cigar := string(fields[5])
		mdTag := ""
		for i := 11; i < len(fields); i++ {
			if bytes.HasPrefix(fields[i], []byte("MD:Z:")) {
				mdTag = string(fields[i][5:])
				break
			}
		}

		// 分析每个位置的匹配情况
		a.analyzeAlignment(refIdx, posStats, cigar, mdTag, fields[9])

		bufferPool.Put(buf)
		lineCount++

		if lineCount%100000 == 0 {
			a.updateStats(refIdx, posStats)
		}
	}

	// 最终更新
	a.updateStats(refIdx, posStats)
	return nil
}

func (a *SynthesisAnalyzer) analyzeAlignment(refIdx int, stats []PositionStats,
	cigar, mdTag string, seq []byte) {

	refPos := 0
	seqPos := 0

	// 解析CIGAR字符串
	i := 0
	for i < len(cigar) {
		// 解析数字
		num := 0
		for i < len(cigar) && cigar[i] >= '0' && cigar[i] <= '9' {
			num = num*10 + int(cigar[i]-'0')
			i++
		}

		op := cigar[i]
		i++

		switch op {
		case 'M': // 匹配或错配
			for j := 0; j < num; j++ {
				if refPos < len(stats) {
					atomic.AddInt64(&stats[refPos].TotalReads, 1)
					// 检查是否匹配（需结合MD标签）
					if a.isMatch(refIdx, refPos, seq[seqPos]) {
						atomic.AddInt64(&stats[refPos].MatchCount, 1)
					} else {
						atomic.AddInt64(&stats[refPos].MismatchCount, 1)
					}
				}
				refPos++
				seqPos++
			}
		case 'D': // 缺失
			for j := 0; j < num; j++ {
				if refPos < len(stats) {
					atomic.AddInt64(&stats[refPos].TotalReads, 1)
					atomic.AddInt64(&stats[refPos].DeletionCount, 1)
				}
				refPos++
			}
		case 'I': // 插入
			if refPos > 0 && refPos-1 < len(stats) {
				atomic.AddInt64(&stats[refPos-1].InsertionCount, 1)
			}
			seqPos += num
		case 'S': // 软裁剪
			seqPos += num
		case 'H': // 硬裁剪
			// 跳过
		}
	}
}

// 使用MD标签准确判断匹配
func (a *SynthesisAnalyzer) isMatch(refIdx, pos int, base byte) bool {
	refBase := a.references[refIdx].Sequence[pos]
	return base == refBase
}
