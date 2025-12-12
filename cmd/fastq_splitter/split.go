package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	gzip "github.com/klauspost/pgzip"
)

// 主拆分函数
func (s *RegexpSplitter) splitReads() error {
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
func (s *RegexpSplitter) processGzippedFile(filename string,
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
func (s *RegexpSplitter) processRecord(record []byte,
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
func (s *RegexpSplitter) reverseComplementRecord(record []byte) []byte {
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
func (s *RegexpSplitter) updateDirectionStats(direction string) {
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
func (s *RegexpSplitter) printDetailedStats() {
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

// 修改主拆分函数，只输出靶标间序列
func (s *RegexpSplitter) splitExtractTargetOnlyEnhanced() error {
	fmt.Println("\n开始拆分（增强版，只保留靶标间序列）...")

	startTime := time.Now()

	// 为每个样本创建输出文件
	writers := make(map[string]*gzip.Writer)
	files := make(map[string]*os.File)

	for _, sample := range s.samples {
		outputFile := filepath.Join(sample.OutputPath, "target_only_reads.fastq.gz")
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("创建输出文件失败: %v", err)
		}
		files[sample.Name] = f
		writers[sample.Name] = gzip.NewWriter(f)
	}

	defer func() {
		// 关闭所有写入器
		for name, writer := range writers {
			writer.Close()
			if f, ok := files[name]; ok {
				f.Close()
			}
		}
	}()

	// 处理所有合并文件
	totalProcessed := 0
	totalMatched := 0

	for mergedFile, samples := range s.fileMap {
		fmt.Printf("处理文件: %s (%d个样本)\n", filepath.Base(mergedFile), len(samples))

		// 使用工作池处理
		matched, processed, err := s.processFileEnhanced(mergedFile, writers)
		if err != nil {
			return fmt.Errorf("处理文件失败: %v", err)
		}

		totalProcessed += processed
		totalMatched += matched

		fmt.Printf("  本文件: 处理 %d 条, 提取 %d 条 (%.1f%%)\n",
			processed, matched, float64(matched)/float64(processed)*100)

		// 记录每个样本的统计
		for _, sample := range samples {
			sample.TotalReads = processed
			if sample.MatchedReads > 0 {
				fmt.Printf("    样本 %s: %d 条\n", sample.Name, sample.MatchedReads)
			}
		}
	}

	elapsed := time.Since(startTime)

	fmt.Printf("\n拆分完成!\n")
	fmt.Printf("总处理: %d 条reads\n", totalProcessed)
	fmt.Printf("总提取: %d 条reads (%.1f%%)\n",
		totalMatched, float64(totalMatched)/float64(totalProcessed)*100)
	fmt.Printf("耗时: %v\n", elapsed)

	// 打印各样本统计
	s.printEnhancedStats()

	return nil
}

// 处理文件，提取靶标间序列
func (s *RegexpSplitter) processFileEnhanced(filename string,
	writers map[string]*gzip.Writer) (int, int, error) {

	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, 0, err
	}
	defer gzReader.Close()

	// 创建工作池
	numWorkers := min(s.config.Threads, 16) // 限制最大工作线程数
	recordChan := make(chan []byte, 10000)
	resultChan := make(chan *MatchResult, 10000)

	var wg sync.WaitGroup

	// 启动worker
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go s.regexpWorker(i, recordChan, resultChan, &wg)
	}

	// 启动读取器
	scanner := bufio.NewScanner(gzReader)
	var record bytes.Buffer
	lineCount := 0
	processedCount := 0
	matchedCount := 0

	// 设置缓冲区大小
	const maxCapacity = 4 * 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// 统计各种匹配方法的成功率
	matchMethodStats := map[string]int{
		"regexp":   0,
		"fuzzy":    0,
		"position": 0,
	}

	// 读取并发送记录到channel
	go func() {
		for scanner.Scan() {
			line := scanner.Bytes()

			if lineCount == 0 {
				if record.Len() > 0 {
					recordData := make([]byte, len(record.Bytes()))
					copy(recordData, record.Bytes())
					recordChan <- recordData
					record.Reset()
					processedCount++
				}

				if len(line) > 0 && line[0] == '@' {
					record.Write(line)
					record.WriteByte('\n')
					lineCount = 1
				}
			} else {
				record.Write(line)
				record.WriteByte('\n')
				lineCount = (lineCount + 1) % 4
			}
		}

		// 最后一个记录
		if record.Len() > 0 {
			recordData := make([]byte, len(record.Bytes()))
			copy(recordData, record.Bytes())
			recordChan <- recordData
			processedCount++
		}

		close(recordChan)
	}()

	// 等待worker完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果并写入文件
	for result := range resultChan {
		if result.sample != nil && result.processedRecord != nil {
			if writer, ok := writers[result.sample.Name]; ok {
				writer.Write(result.processedRecord)
				matchedCount++

				// 更新统计
				result.sample.MatchedReads++
				matchMethodStats[result.method]++

				// 记录提取的序列长度
				if result.sample.MinExtractedLen == 0 || len(result.processedRecord) < result.sample.MinExtractedLen {
					result.sample.MinExtractedLen = len(result.processedRecord)
				}
				if len(result.processedRecord) > result.sample.MaxExtractedLen {
					result.sample.MaxExtractedLen = len(result.processedRecord)
				}
				result.sample.TotalExtractedLen += len(result.processedRecord)
			}
		}
	}
	// 输出匹配方法统计
	fmt.Printf("    匹配方法统计: ")
	for method, count := range matchMethodStats {
		if count > 0 {
			fmt.Printf("%s=%d (%.1f%%) ",
				method, count, float64(count)/float64(matchedCount)*100)
		}
	}
	fmt.Println()
	return matchedCount, processedCount, scanner.Err()
}

// 匹配结果
type MatchResult struct {
	sample          *SampleInfo
	method          string
	processedRecord []byte
}

// regexp worker
func (s *RegexpSplitter) regexpWorker(id int, recordChan <-chan []byte,
	resultChan chan<- *MatchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for record := range recordChan {
		sample, method, processedRecord := s.processRecordEnhanced(record)
		resultChan <- &MatchResult{
			sample:          sample,
			method:          method,
			processedRecord: processedRecord,
		}
	}
}

// 使用预编译的正则表达式池
type RegexpPool struct {
	regexps []*regexp.Regexp
	samples []*SampleInfo
}

// 构建优化的正则表达式
func (s *RegexpSplitter) buildTargetOnlyRegexp() error {
	fmt.Println("构建只保留靶标间序列的正则表达式...")

	// 为所有样本创建正则表达式
	for _, sample := range s.samples {
		// 构建完整参考序列（用于后续分析）
		sample.FullReference = sample.TargetSeq + sample.SynthesisSeq + sample.PostTargetSeq

		// 计算反向互补序列
		sample.ReverseTarget = reverseComplement(sample.TargetSeq)
		sample.ReversePostTarget = reverseComplement(sample.PostTargetSeq)

		// 1. 正向正则表达式：捕获头靶标和尾靶标之间的序列（包括头尾靶标）
		// 使用分组捕获：头靶标(.*?)尾靶标
		forwardPattern := `(` + s.escapeRegexp(sample.TargetSeq) + `.*?` + s.escapeRegexp(sample.PostTargetSeq) + `)`
		forwardRegex, err := regexp.Compile(forwardPattern)
		if err != nil {
			return fmt.Errorf("构建正向正则表达式失败（样本%s）: %v", sample.Name, err)
		}

		seqRegexp := &SeqRegexp{
			sample:       sample,
			forwardRegex: forwardRegex,
		}

		// 2. 反向正则表达式
		if s.useRC {
			// 反向互补序列：尾靶标在前，头靶标在后
			reversePattern := `(` + s.escapeRegexp(sample.ReversePostTarget) + `.*?` + s.escapeRegexp(sample.ReverseTarget) + `)`
			reverseRegex, err := regexp.Compile(reversePattern)
			if err != nil {
				return fmt.Errorf("构建反向正则表达式失败（样本%s）: %v", sample.Name, err)
			}

			seqRegexp.reverseRegex = reverseRegex

			fmt.Printf("  样本 %s:\n", sample.Name)
			fmt.Printf("    正向模式: %s\n", forwardPattern)
			fmt.Printf("    反向模式: %s\n", reversePattern)
		}

		s.seqRegexps = append(s.seqRegexps, seqRegexp)
	}

	return nil
}

// 优化的匹配函数
func (s *RegexpSplitter) optimizedMatch(sequence string) (*SampleInfo, string) {
	// 快速检查：如果序列太短，跳过
	if len(sequence) < 50 {
		return nil, ""
	}

	// 尝试正向匹配
	for _, seqRegexp := range s.seqRegexps {
		if seqRegexp.forwardRegex.MatchString(sequence) {
			// 验证匹配位置
			headIdx := strings.Index(sequence, seqRegexp.sample.TargetSeq)
			tailIdx := strings.Index(sequence, seqRegexp.sample.PostTargetSeq)

			if headIdx != -1 && tailIdx != -1 && headIdx < tailIdx {
				// 确保头靶标在序列的前半部分
				if headIdx < len(sequence)/2 {
					return seqRegexp.sample, "forward"
				}
			}
		}
	}

	// 尝试反向匹配
	if s.useRC {
		for _, seqRegexp := range s.seqRegexps {
			if seqRegexp.reverseRegex != nil && seqRegexp.reverseRegex.MatchString(sequence) {
				rcTarget := reverseComplement(seqRegexp.sample.TargetSeq)
				rcPostTarget := reverseComplement(seqRegexp.sample.PostTargetSeq)

				headIdx := strings.Index(sequence, rcTarget)
				tailIdx := strings.Index(sequence, rcPostTarget)

				if headIdx != -1 && tailIdx != -1 && headIdx < tailIdx {
					if headIdx < len(sequence)/2 {
						return seqRegexp.sample, "reverse"
					}
				}
			}
		}
	}

	return nil, ""
}

// 增强的统计函数
func (s *RegexpSplitter) printEnhancedStats() {
	fmt.Println("\n增强版靶标提取统计:")
	fmt.Println(strings.Repeat("=", 100))

	totalReads := 0
	totalExtracted := 0
	totalLength := 0

	fmt.Printf("  样本名称                 处理数    提取数    提取率%%   最短长度   最长长度   平均长度   正向%%   反向%%\n")
	fmt.Println(strings.Repeat("-", 100))

	for _, sample := range s.samples {
		totalReads += sample.TotalReads
		totalExtracted += sample.MatchedReads
		totalLength += sample.TotalExtractedLen

		extractionRate := 0.0
		avgLength := 0.0
		forwardRate := 0.0

		if sample.TotalReads > 0 {
			extractionRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		if sample.MatchedReads > 0 {
			avgLength = float64(sample.TotalExtractedLen) / float64(sample.MatchedReads)
			forwardRate = float64(sample.ForwardReads) / float64(sample.MatchedReads) * 100
		}

		fmt.Printf("  %-20s  %8d  %8d  %8.1f  %8d  %8d  %8.1f  %6.1f  %6.1f\n",
			sample.Name,
			sample.TotalReads,
			sample.MatchedReads,
			extractionRate,
			sample.MinExtractedLen,
			sample.MaxExtractedLen,
			avgLength,
			forwardRate,
			100-forwardRate,
		)
	}

	fmt.Println(strings.Repeat("-", 100))

	overallRate := 0.0
	avgOverallLength := 0.0

	if totalReads > 0 {
		overallRate = float64(totalExtracted) / float64(totalReads) * 100
	}
	if totalExtracted > 0 {
		avgOverallLength = float64(totalLength) / float64(totalExtracted)
	}

	fmt.Printf("  总计                  %8d  %8d  %8.1f  %8s  %8s  %8.1f  %6s  %6s\n",
		totalReads,
		totalExtracted,
		overallRate,
		"-",
		"-",
		avgOverallLength,
		"-",
		"-")

	// 输出每个样本的靶标信息
	fmt.Println("\n各样本靶标信息:")
	fmt.Println(strings.Repeat("=", 80))
	for _, sample := range s.samples {
		fullRef := sample.TargetSeq + sample.SynthesisSeq + sample.PostTargetSeq
		fmt.Printf("  样本 %-20s: 头靶标(%d) + 合成序列(%d) + 尾靶标(%d) = 总长 %d\n",
			sample.Name,
			len(sample.TargetSeq),
			len(sample.SynthesisSeq),
			len(sample.PostTargetSeq),
			len(fullRef))
	}
}

// 调试模式：输出未匹配的序列
func (s *RegexpSplitter) debugUnmatched(filename string, outputDir string) error {
	debugFile := filepath.Join(outputDir, "unmatched_sequences.txt")
	f, err := os.Create(debugFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	scanner := bufio.NewScanner(gzReader)
	var record bytes.Buffer
	lineCount := 0
	unmatchedCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()

		if lineCount == 0 {
			if record.Len() > 0 {
				recordStr := record.String()
				lines := strings.Split(recordStr, "\n")
				if len(lines) > 1 {
					sequence := lines[1]

					// 尝试匹配
					sample, _ := s.optimizedMatch(sequence)
					if sample == nil {
						// 未匹配，记录调试信息
						writer.WriteString(fmt.Sprintf("未匹配序列 %d:\n", unmatchedCount+1))
						writer.WriteString(fmt.Sprintf("序列: %s\n", sequence))
						writer.WriteString("可能的匹配检查:\n")

						// 检查每个样本的匹配情况
						for _, testSample := range s.samples {
							if strings.Contains(sequence, testSample.TargetSeq) {
								writer.WriteString(fmt.Sprintf("  包含头靶标: %s (样本: %s)\n",
									testSample.TargetSeq, testSample.Name))
							}
							if strings.Contains(sequence, testSample.PostTargetSeq) {
								writer.WriteString(fmt.Sprintf("  包含尾靶标: %s (样本: %s)\n",
									testSample.PostTargetSeq, testSample.Name))
							}
						}
						writer.WriteString("\n")
						unmatchedCount++
					}
				}
				record.Reset()
			}

			if len(line) > 0 && line[0] == '@' {
				record.Write(line)
				record.WriteByte('\n')
				lineCount = 1
			}
		} else {
			record.Write(line)
			record.WriteByte('\n')
			lineCount = (lineCount + 1) % 4
		}
	}

	fmt.Printf("调试完成: 找到 %d 条未匹配序列，详见 %s\n", unmatchedCount, debugFile)
	return nil
}
