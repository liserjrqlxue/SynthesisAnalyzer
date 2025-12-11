package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	gzip "github.com/klauspost/pgzip"
)

// 主拆分函数
func (s *ExactMatchSplitter) splitReads() error {
	fmt.Println("\n步骤6: 开始拆分（完全匹配，支持反向互补）...")

	// 重置统计
	s.stats = struct {
		totalReads     int64
		matchedReads   int64
		ambiguousReads int64
		failedReads    int64
		forwardMatches int64
		reverseMatches int64
	}{}

	totalSamples := 0

	// 为每个样本创建输出文件
	writers := make(map[string]*gzip.Writer)
	files := make(map[string]*os.File)

	for _, sample := range s.samples {
		outputFile := filepath.Join(sample.OutputPath, "split_reads.fastq.gz")
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("创建输出文件失败: %v", err)
		}
		files[sample.Name] = f
		writers[sample.Name] = gzip.NewWriter(f)
	}

	defer func() {
		// 关闭所有写入器
		for sampleName, writer := range writers {
			writer.Close()
			if f, ok := files[sampleName]; ok {
				f.Close()
			}
			totalSamples++
		}
		fmt.Printf("\n  总计: %d 个样品, %d 条reads\n", totalSamples, s.stats.totalReads)
	}()

	// 处理所有合并文件
	for mergedFile, samples := range s.fileMap {
		fmt.Printf("\n  处理合并文件: %s (%d个样品)\n",
			filepath.Base(mergedFile), len(samples))

		// 读取.gz文件
		stats, err := s.processGzippedFile(mergedFile, writers)
		if err != nil {
			return fmt.Errorf("处理文件失败: %v", err)
		}

		// 更新统计
		for sampleName, count := range stats {
			// s.stats.totalReads += int64(count)
			// 更新样本的read count
			for _, sample := range s.samples {
				if sample.Name == sampleName {
					sample.ReadCount += count
					break
				}
			}
		}
		fmt.Printf("  本文件匹配: %d 条reads\n", sumMap(stats))

	}

	// 打印详细统计
	s.printDetailedStats()

	return nil
}

// 处理gzip压缩的FASTQ文件
func (s *ExactMatchSplitter) processGzippedFile(filename string,
	writers map[string]*gzip.Writer) (map[string]int, error) {
	stats := make(map[string]int)

	// 打开gz文件
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	scanner := bufio.NewScanner(gzReader)
	var record bytes.Buffer
	lineCount := 0
	batchCount := 0
	batchSize := 10000
	totalReads := 0

	// 设置缓冲区大小以提高性能
	const maxCapacity = 4 * 1024 * 1024 // 4MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()

		if lineCount == 0 {
			// 处理前一个记录
			if record.Len() > 0 {
				recordData := record.Bytes()
				sample, direction := s.processRecord(recordData, writers)
				if sample != nil {
					stats[sample.Name]++
					s.updateDirectionStats(direction)
				}
				record.Reset()
				batchCount++
				// 定期输出进度
				if batchCount%batchSize == 0 {
					fmt.Printf("  已处理: %d / %d 条reads\n", s.stats.matchedReads, batchCount)
				}
			}

			// 新记录开始
			if len(line) > 0 && line[0] == '@' {
				record.Write(line)
				record.WriteByte('\n')
				lineCount = 1
				totalReads++
				s.statsMutex.Lock()
				s.stats.totalReads++
				s.statsMutex.Unlock()
			}
		} else {
			// 继续当前记录
			record.Write(line)
			record.WriteByte('\n')
			lineCount = (lineCount + 1) % 4
		}
	}

	// 处理最后一个记录
	if record.Len() > 0 {
		recordData := record.Bytes()
		sample, direction := s.processRecord(recordData, writers)
		if sample != nil {
			stats[sample.Name]++
			s.updateDirectionStats(direction)
		}
		batchCount++
	}
	return stats, scanner.Err()
}

// 处理单个记录
func (s *ExactMatchSplitter) processRecord(record []byte,
	writers map[string]*gzip.Writer) (*SampleInfo, string) {

	// 解析FASTQ记录，提取序列
	lines := bytes.SplitN(record, []byte{'\n'}, 4)
	if len(lines) < 2 {
		return nil, ""
	}

	sequence := string(lines[1])

	// 完全匹配
	sample, direction := s.exactMatch(sequence)
	if sample == nil {
		s.statsMutex.Lock()
		s.stats.failedReads++
		s.statsMutex.Unlock()
		return nil, ""
	}

	// 写入到对应文件
	if writer, ok := writers[sample.Name]; ok {
		// 如果是反向匹配，需要将序列反向互补
		if direction == "reverse" {
			// 对整个记录进行反向互补处理
			record = s.reverseComplementRecord(record)
		}

		writer.Write(record)
		s.statsMutex.Lock()
		s.stats.matchedReads++
		s.statsMutex.Unlock()

		return sample, direction
	}

	return nil, ""
}

// 对整个FASTQ记录进行反向互补
func (s *ExactMatchSplitter) reverseComplementRecord(record []byte) []byte {
	lines := bytes.SplitN(record, []byte{'\n'}, 4)
	if len(lines) < 4 {
		return record
	}

	// 反向互补序列
	sequence := string(lines[1])
	rcSequence := reverseComplement(sequence)

	// 反转质量值（因为序列反向了，质量值也要对应反转）
	quality := string(lines[3])
	rcQuality := reverseString(quality)

	// 重建记录
	var result bytes.Buffer
	result.Write(lines[0]) // header
	result.WriteByte('\n')
	result.WriteString(rcSequence)
	result.WriteByte('\n')
	result.Write(lines[2]) // plus line
	result.WriteByte('\n')
	result.WriteString(rcQuality)
	result.WriteByte('\n')

	return result.Bytes()
}

// 反转字符串
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// 更新方向统计
func (s *ExactMatchSplitter) updateDirectionStats(direction string) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	switch direction {
	case "forward":
		s.stats.forwardMatches++
	case "reverse":
		s.stats.reverseMatches++
	}
}

// 打印详细统计
func (s *ExactMatchSplitter) printDetailedStats() {
	fmt.Println("\n拆分统计:")
	// fmt.Println("=" * 50)

	total := s.stats.totalReads
	matched := s.stats.matchedReads
	failed := s.stats.failedReads
	ambiguous := s.stats.ambiguousReads
	forward := s.stats.forwardMatches
	reverse := s.stats.reverseMatches

	fmt.Printf("总读取数:      %d\n", total)
	fmt.Printf("成功匹配:      %d (%.1f%%)\n",
		matched, float64(matched)/float64(total)*100)
	fmt.Printf("  - 正向匹配:  %d (%.1f%%)\n",
		forward, float64(forward)/float64(matched)*100)
	fmt.Printf("  - 反向匹配:  %d (%.1f%%)\n",
		reverse, float64(reverse)/float64(matched)*100)
	fmt.Printf("匹配失败:      %d (%.1f%%)\n",
		failed, float64(failed)/float64(total)*100)
	fmt.Printf("歧义匹配:      %d (%.1f%%)\n",
		ambiguous, float64(ambiguous)/float64(total)*100)

	// 每个样本的统计
	fmt.Println("\n各样本统计:")
	// fmt.Println("-" * 50)
	for _, sample := range s.samples {
		if sample.ReadCount > 0 {
			fmt.Printf("  %-20s: %d reads\n", sample.Name, sample.ReadCount)
		}
	}
}

func sumMap(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}
