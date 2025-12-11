package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	// "compress/gzip"

	gzip "github.com/klauspost/pgzip"
	"github.com/tealeg/xlsx"
)

func main() {
	// 检查参数
	if len(os.Args) < 3 {
		fmt.Println("用法: fastq_splitter <Excel文件> [输出目录] [Fastq目录]")
		fmt.Println("示例: fastq_splitter samples.xlsx ./output ./fastq")
		os.Exit(1)
	}

	excelFile := os.Args[1]
	outputDir := "./output"
	fastqDir := ""
	if len(os.Args) > 2 {
		outputDir = os.Args[2]
	}
	if len(os.Args) > 3 {
		fastqDir = os.Args[3]
	}

	// 创建配置
	config := &Config{
		ExcelFile:    excelFile,
		OutputDir:    outputDir,
		FastqDir:     fastqDir,
		Threads:      runtime.NumCPU(),
		SearchWindow: 200, // 从头/尾搜索50bp
		Quality:      20,
		MergeLen:     80,

		UseRC:         true, // 启用反向互补匹配
		SkipExisting:  true, // 默认跳过已存在文件
		Compression:   true, // 默认启用压缩
		CompressLevel: 6,    // 默认压缩级别
		CleanupTemp:   true, // 不保留临时文件

		AllowMismatch:  2,             // 允许2个错配
		MatchThreshold: 30,            // 匹配分数阈值
		OutputMode:     "target-only", // 只输出靶标间序列

	}

	// 创建处理器
	splitter := NewRegexpSplitter(config)
	fmt.Printf("%+v", splitter)

	// 运行处理流程
	if err := splitter.Run(); err != nil {
		log.Fatalf("处理失败: %v", err)
	}

	fmt.Println("处理完成!")
}

// 主运行流程
func (s *RegexpSplitter) Run() error {
	startTime := time.Now()
	fmt.Println("=== FASTQ正则表达式拆分程序开始运行 ===")

	// 步骤1: 创建输出目录
	if err := s.createOutputDir(); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 步骤2: 读取Excel文件
	if err := s.readExcel(); err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}

	// 步骤3: 检查重复样品名称
	if err := s.checkDuplicates(); err != nil {
		return err
	}

	// 步骤4: 合并PE reads
	fmt.Println("\n步骤4: 合并PE reads...")
	if err := s.mergeReads(); err != nil {
		return fmt.Errorf("合并reads失败: %v", err)
	}

	// 步骤5: 构建索引
	fmt.Println("\n步骤5: 构建完全匹配索引...")
	// if err := s.buildIndex(); err != nil {
	// 5. 构建正则表达式索引
	if err := s.buildTargetOnlyRegexp(); err != nil {
		return fmt.Errorf("构建索引失败: %v", err)
	}

	// 步骤6: 拆分reads
	fmt.Println("\n步骤6: 拆分reads...")
	// 使用简单版本避免mmap问题
	// if err := s.splitReads(); err != nil {
	// 6. 使用正则表达式拆分
	if err := s.splitExtractTargetOnly(); err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}

	// 步骤7: 生成报告
	fmt.Println("\n步骤7: 生成报告...")
	if err := s.generateExtractionReport(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	// 8. 可选：调试模式（输出未匹配的序列）
	if os.Getenv("DEBUG") == "1" {
		for mergedFile := range s.fileMap {
			s.debugUnmatched(mergedFile, s.config.OutputDir)
			break // 只调试第一个文件
		}
	}

	fmt.Printf("\n=== 全部处理完成! 总耗时: %v ===\n", time.Since(startTime))
	return nil
}

// 创建输出目录
func (s *RegexpSplitter) createOutputDir() error {
	if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
		return err
	}

	// 创建日志目录
	logDir := filepath.Join(s.config.OutputDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 设置日志输出
	logFile := filepath.Join(logDir, "splitter.log")
	f, err := os.Create(logFile)
	if err != nil {
		return err
	}

	// 同时输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, f)
	log.SetOutput(multiWriter)

	return nil
}

// 读取Excel文件
func (s *RegexpSplitter) readExcel() error {
	fmt.Printf("步骤1: 读取Excel文件: %s\n", s.config.ExcelFile)

	xlFile, err := xlsx.OpenFile(s.config.ExcelFile)
	if err != nil {
		return fmt.Errorf("无法打开Excel文件: %v", err)
	}

	if len(xlFile.Sheets) == 0 {
		return fmt.Errorf("Excel文件中没有工作表")
	}

	sheet := xlFile.Sheets[0]
	fmt.Printf("工作表: %s, 行数: %d\n", sheet.Name, sheet.MaxRow)

	// 查找表头
	var headerMap = make(map[string]int)
	for col := 0; col < sheet.MaxCol; col++ {
		cell := sheet.Cell(0, col)
		headerMap[cell.Value] = col
	}

	// 检查必需列
	requiredCols := []string{"样品名称", "靶标序列", "合成序列", "后靶标", "路径-R1", "路径-R2"}
	for _, col := range requiredCols {
		if _, ok := headerMap[col]; !ok {
			return fmt.Errorf("缺少必需的列: %s", col)
		}
	}

	// 读取数据行
	for row := 1; row < sheet.MaxRow; row++ {
		sample := &SampleInfo{
			Name:          sheet.Cell(row, headerMap["样品名称"]).Value,
			TargetSeq:     strings.ToUpper(sheet.Cell(row, headerMap["靶标序列"]).Value),
			SynthesisSeq:  strings.ToUpper(sheet.Cell(row, headerMap["合成序列"]).Value),
			PostTargetSeq: strings.ToUpper(sheet.Cell(row, headerMap["后靶标"]).Value),
			R1Path:        sheet.Cell(row, headerMap["路径-R1"]).Value,
			R2Path:        sheet.Cell(row, headerMap["路径-R2"]).Value,
		}

		// 检查必需字段
		if sample.Name == "" {
			continue // 跳过空行
		}

		// 创建输出目录
		sample.OutputPath = filepath.Join(s.config.OutputDir, sample.Name)
		if err := os.MkdirAll(sample.OutputPath, 0755); err != nil {
			return fmt.Errorf("创建样品目录失败: %v", err)
		}

		s.samples = append(s.samples, sample)

		fmt.Printf("  读取样品: %s, R1: %s, R2: %s\n",
			sample.Name, filepath.Base(sample.R1Path), filepath.Base(sample.R2Path))
	}

	fmt.Printf("  成功读取 %d 个样品\n", len(s.samples))
	return nil
}

// 检查重复样品名称
func (s *RegexpSplitter) checkDuplicates() error {
	fmt.Println("\n步骤2: 检查重复样品名称...")

	seen := make(map[string]bool)
	duplicates := []string{}

	for _, sample := range s.samples {
		if seen[sample.Name] {
			duplicates = append(duplicates, sample.Name)
		} else {
			seen[sample.Name] = true
		}
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("发现重复的样品名称: %v", duplicates)
	}

	fmt.Println("  所有样品名称唯一，通过检查")
	return nil
}

// 合并PE reads
func (s *RegexpSplitter) mergeReads() error {
	// 去重R1/R2文件对
	filePairs := make(map[string][]*SampleInfo) // key: "R1|R2" -> 样品列表

	for _, sample := range s.samples {
		key := sample.R1Path + "|" + sample.R2Path
		filePairs[key] = append(filePairs[key], sample)
	}

	fmt.Printf("  需要合并 %d 个唯一的R1/R2文件对\n", len(filePairs))

	// 创建合并目录
	mergedDir := filepath.Join(s.config.OutputDir, "merged")
	if err := os.MkdirAll(mergedDir, 0755); err != nil {
		return fmt.Errorf("创建合并目录失败: %v", err)
	}

	// 并行合并
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.config.Threads/2) // 限制并发数
	results := make(chan struct {
		key    string
		output string
		err    error
	}, len(filePairs))

	var mergedCount int64
	var skippedCount int64

	for key, samples := range filePairs {
		wg.Add(1)
		sem <- struct{}{}

		go func(k string, samps []*SampleInfo) {
			defer func() {
				<-sem
				wg.Done()
			}()

			// 生成合并后的文件名
			mergedFile, exists := s.getMergedFileName(samps[0])

			result := struct {
				key    string
				output string
				err    error
			}{
				key:    k,
				output: mergedFile,
				err:    nil,
			}

			if exists && s.config.SkipExisting {
				atomic.AddInt64(&skippedCount, 1)
				results <- result
				return
			}

			// 运行fastp
			output, err := s.runFastpWithCheck(samps[0].R1Path, samps[0].R2Path, mergedFile)
			result.output = output
			result.err = err
			if err == nil {
				atomic.AddInt64(&mergedCount, 1)
				// 创建完成标签
				if err := s.createDoneFile(output); err != nil {
					log.Printf("警告: 创建完成标签失败: %v", err)
				}

				// 更新样品信息
				for _, sample := range samps {
					sample.MergeTime = time.Now()
				}
			}

			results <- result
		}(key, samples)
	}

	// 等待所有合并完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	errorCount := 0
	for result := range results {
		if result.err != nil {
			errorCount++
			log.Printf("合并失败 (R1: %s): %v", result.key, result.err)
			continue
		}

		// 更新对应样品的MergedFile字段
		samples := filePairs[result.key]
		for _, sample := range samples {
			sample.Status = "merged"
			sample.MergedFile = result.output
			s.fileMap[result.output] = append(s.fileMap[result.output], sample)
		}

		fmt.Printf("  合并完成: %s -> %s (%d个样品)\n",
			result.key, filepath.Base(result.output), len(samples))
	}

	fmt.Printf("  合并结果: %d 个已处理, %d 个已跳过, %d 个失败\n",
		mergedCount, skippedCount, errorCount)

	// 检查是否有样品合并失败
	for _, sample := range s.samples {
		if sample.MergedFile == "" {
			return fmt.Errorf("样品 %s 合并失败，请检查输入文件", sample.Name)
		}
	}

	return nil
}

// 生成合并文件名
func (s *RegexpSplitter) getMergedFileName(sample *SampleInfo) (string, bool) {
	// 基于R1和R2路径生成唯一的文件名
	base1 := filepath.Base(sample.R1Path)
	base2 := filepath.Base(sample.R2Path)

	// 去掉扩展名
	base1 = strings.TrimSuffix(base1, filepath.Ext(base1))
	base2 = strings.TrimSuffix(base2, filepath.Ext(base2))

	var base []byte
	for i := 0; i < min(len(base1), len(base2)); i++ {
		if base1[i] == base2[i] {
			base = append(base, base1[i])
		} else {
			break
		}
	}

	// 合并目录
	mergedDir := filepath.Join(s.config.OutputDir, "merged")
	os.MkdirAll(mergedDir, 0755)

	// 使用第一个样品的名称作为文件名
	filename := fmt.Sprintf("%s_merged.fastq.gz", sample.Name)
	if len(base) > 0 {
		filename = fmt.Sprintf("%smerged.fastq.gz", base)
	}
	mergedFile := filepath.Join(mergedDir, filename)
	doneFile := mergedFile + ".done"

	// 检查合并文件和完成标签是否都存在
	if _, err1 := os.Stat(mergedFile); err1 == nil {
		if _, err2 := os.Stat(doneFile); err2 == nil {
			// 检查文件大小是否合理（至少1KB）
			if fi, err := os.Stat(mergedFile); err == nil && fi.Size() > 1024 {
				fmt.Printf("    合并文件已存在且完成标签存在: %s\n", filepath.Base(mergedFile))
				return mergedFile, true
			}
		}
	}

	return mergedFile, false
}

// 创建完成标签
func (s *RegexpSplitter) createDoneFile(filename string) error {
	doneFile := filename + ".done"
	content := fmt.Sprintf("Created: %s\nFile: %s\n",
		time.Now().Format(time.RFC3339),
		filepath.Base(filename))

	return os.WriteFile(doneFile, []byte(content), 0644)
}

// 运行fastp进行合并
func (s *RegexpSplitter) runFastp(r1Path, r2Path, outputFile string) (string, error) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("fastp_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %v", err)
	}

	defer func() {
		if !s.config.CleanupTemp {
			fmt.Printf("    临时文件保留在: %s\n", tempDir)
			return
		}
		// 清理临时目录
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("警告: 清理临时目录失败: %v", err)
		}
	}()

	if s.config.FastqDir != "" {
		r1Path = filepath.Join(s.config.FastqDir, r1Path)
		r2Path = filepath.Join(s.config.FastqDir, r2Path)
	}
	// 检查文件是否存在
	if _, err := os.Stat(r1Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R1文件不存在: %s", r1Path)
	}
	if _, err := os.Stat(r2Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R2文件不存在: %s", r2Path)
	}

	// 构建fastp命令
	args := s.buildFastpCommand(r1Path, r2Path, outputFile, tempDir)
	cmd := exec.Command("fastp", args...)

	// 捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置超时（30分钟）
	timeout := 30 * time.Minute
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动fastp失败: %v", err)
	}

	log.Printf("CMD:\n%s", cmd)
	// 等待完成或超时
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return "", fmt.Errorf("fastp执行超时(%v)", timeout)
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("fastp执行失败: %v\n%s", err, stderr.String())
		}
	}

	// 检查输出
	if fi, err := os.Stat(outputFile); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("合并文件生成失败或为空")
	}

	// 读取合并统计
	stats, _ := s.readFastpStats(outputFile + ".fastp.json")
	fmt.Printf("    合并完成: %s,%s -> %s (%.2f MB)\n\t(统计: %v)\n",
		filepath.Base(r1Path),
		filepath.Base(r2Path),
		filepath.Base(outputFile),
		float64(getFileSize(outputFile))/1024/1024,
		stats,
	)

	return outputFile, nil
}

// 构建fastp命令
func (s *RegexpSplitter) buildFastpCommand(r1Path, r2Path, outputFile, tempDir string) []string {
	args := []string{
		"-i", r1Path,
		"-I", r2Path,
		"--merged_out", outputFile,
		"--thread", "4",
		"--merge",
		"--overlap_len_require", "20",
		"--overlap_diff_limit", "5",
		"--qualified_quality_phred", fmt.Sprintf("%d", s.config.Quality),
		"--length_required", fmt.Sprintf("%d", s.config.MergeLen-50),
		"--correction",
		"--detect_adapter_for_pe",
		"--html", outputFile + ".fastp.html",
		"--json", outputFile + ".fastp.json",
	}

	// 添加压缩选项
	if strings.HasSuffix(outputFile, ".gz") {
		args = append(args, "--compression", fmt.Sprintf("%d", s.config.CompressLevel))
	} else {
		args = append(args, "--uncompressed")
	}

	// 设置临时输出文件
	out1File := filepath.Join(tempDir, "out1.fastq")
	out2File := filepath.Join(tempDir, "out2.fastq")
	args = append(args, "--out1", out1File, "--out2", out2File)

	return args
}

// 带检查的fastp运行函数
func (s *RegexpSplitter) runFastpWithCheck(r1Path, r2Path, outputFile string) (string, error) {
	// 确保输出文件以.gz结尾
	if !strings.HasSuffix(outputFile, ".gz") {
		outputFile = outputFile + ".gz"
	}

	fmt.Printf("    开始合并: %s + %s -> %s\n",
		filepath.Base(r1Path), filepath.Base(r2Path), filepath.Base(outputFile))

	startTime := time.Now()

	// 运行fastp
	output, err := s.runFastp(r1Path, r2Path, outputFile)

	if err != nil {
		return "", fmt.Errorf("fastp合并失败: %v", err)
	}

	elapsed := time.Since(startTime)
	fileSize := getFileSize(output)

	fmt.Printf("    合并完成: %s (%.2f MB, 耗时: %v)\n",
		filepath.Base(output), float64(fileSize)/1024/1024, elapsed)

	return output, nil
}

// 获取文件大小
func getFileSize(filename string) int64 {
	fi, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// 读取fastp统计信息
func (s *RegexpSplitter) readFastpStats(jsonFile string) (map[string]interface{}, error) {
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	if summary, ok := result["summary"].(map[string]interface{}); ok {
		if before, ok := summary["before_filtering"].(map[string]interface{}); ok {
			stats["before_reads"] = before["total_reads"]
		}
		if after, ok := summary["after_filtering"].(map[string]interface{}); ok {
			stats["after_reads"] = after["total_reads"]
			stats["merged_reads"] = after["merged_reads"]
		}
	}

	return stats, nil
}

// 使用goroutine管道处理
func (s *RegexpSplitter) splitReadsPipeline() error {
	fmt.Println("使用管道模式处理...")

	totalProcessed := int64(0)
	var wg sync.WaitGroup

	for mergedFile, samples := range s.fileMap {
		fmt.Printf("  处理文件: %s\n", filepath.Base(mergedFile))

		// 创建管道
		recordChan := make(chan []byte, 10000)
		resultChan := make(chan struct {
			sampleName string
			direction  string
			record     []byte
		}, 10000)

		// 启动读取goroutine
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			defer close(recordChan)

			if err := s.readRecordsToChannel(filename, recordChan); err != nil {
				log.Printf("读取文件失败: %v", err)
			}
		}(mergedFile)

		// 启动处理goroutine
		for i := 0; i < runtime.NumCPU(); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for record := range recordChan {
					sequence, found := extractSequence(record)
					if found {
						sample, direction := s.matchSequence(sequence, samples)
						if sample != nil {
							resultChan <- struct {
								sampleName string
								direction  string
								record     []byte
							}{
								sampleName: sample.Name,
								direction:  direction,
								record:     record,
							}
						}
					}
					atomic.AddInt64(&totalProcessed, 1)
				}
			}()
		}

		// 启动写入goroutine
		writers := make(map[string]*gzip.Writer)
		writerFiles := make(map[string]*os.File)

		for _, sample := range samples {
			outputFile := filepath.Join(sample.OutputPath, "split_reads.fastq")
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("创建输出文件失败: %v", err)
			}
			writerFiles[sample.Name] = f
			writers[sample.Name] = gzip.NewWriter(f)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(resultChan)

			stats := make(map[string]int64)
			for result := range resultChan {
				if writer, ok := writers[result.sampleName]; ok {
					writer.Write(result.record)
					stats[result.sampleName]++
				}
			}

			// 刷新并关闭文件
			for sampleName, writer := range writers {
				writer.Close()
				if f, ok := writerFiles[sampleName]; ok {
					f.Close()
				}
				fmt.Printf("    样品 %s: %d reads\n", sampleName, stats[sampleName])
			}
		}()
	}

	wg.Wait()
	fmt.Printf("\n  总计处理: %d 条reads\n", totalProcessed)
	return nil
}

// 将记录读取到channel
func (s *RegexpSplitter) readRecordsToChannel(filename string, recordChan chan<- []byte) error {

	// 打开gz文件
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

	scanner := bufio.NewScanner(file)
	var record bytes.Buffer
	lineCount := 0

	// 设置缓冲区大小以提高性能
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()

		if lineCount == 0 {
			// 处理前一个记录
			if record.Len() > 0 {
				recordCopy := make([]byte, len(record.Bytes()))
				copy(recordCopy, record.Bytes())
				recordChan <- recordCopy
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

	if record.Len() > 0 {
		recordCopy := make([]byte, len(record.Bytes()))
		copy(recordCopy, record.Bytes())
		recordChan <- recordCopy
	}

	return scanner.Err()
}

// 提取序列
func extractSequence(record []byte) ([]byte, bool) {
	lines := bytes.SplitN(record, []byte{'\n'}, 4)
	if len(lines) >= 2 {
		return lines[1], true
	}
	return nil, false
}

// 匹配序列到样品
func (s *RegexpSplitter) matchSequence(sequence []byte, samples []*SampleInfo) (*SampleInfo, string) {
	seqStr := string(sequence)
	// 完全匹配
	sample, direction := s.exactMatch(seqStr)
	if sample == nil {
		s.statsMutex.Lock()
		s.stats.failedReads++
		s.statsMutex.Unlock()
		return nil, ""
	}
	return sample, direction
}

// 反向互补
func reverseComplement(seq string) string {
	var rc strings.Builder
	rc.Grow(len(seq))

	// 从尾部向头部遍历，同时互补
	for i := len(seq) - 1; i >= 0; i-- {
		switch seq[i] {
		case 'A', 'a':
			rc.WriteByte('T')
		case 'T', 't':
			rc.WriteByte('A')
		case 'C', 'c':
			rc.WriteByte('G')
		case 'G', 'g':
			rc.WriteByte('C')
		default:
			rc.WriteByte('N')
		}
	}

	return rc.String()
}

// 生成提取报告
func (s *RegexpSplitter) generateExtractionReport() error {
	reportFile := filepath.Join(s.config.OutputDir, "extraction_report.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"样品名称",
		"头靶标长度",
		"合成序列长度",
		"尾靶标长度",
		"总参考长度",
		"R1文件",
		"R2文件",
		"合并文件",
		"输出目录",
		"处理reads数",
		"提取reads数",
		"提取率%",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range s.samples {
		extractionRate := 0.0
		if sample.TotalReads > 0 {
			extractionRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		record := []string{
			sample.Name,
			fmt.Sprintf("%d", len(sample.TargetSeq)),
			fmt.Sprintf("%d", len(sample.SynthesisSeq)),
			fmt.Sprintf("%d", len(sample.PostTargetSeq)),
			fmt.Sprintf("%d", len(sample.FullReference)),
			filepath.Base(sample.R1Path),
			filepath.Base(sample.R2Path),
			filepath.Base(sample.MergedFile),
			sample.OutputPath,
			fmt.Sprintf("%d", sample.TotalReads),
			fmt.Sprintf("%d", sample.MatchedReads),
			fmt.Sprintf("%.1f", extractionRate),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}
	fmt.Printf("提取报告已生成: %s\n", reportFile)

	// 生成汇总统计
	summaryFile := filepath.Join(s.config.OutputDir, "summary.txt")
	sf, err := os.Create(summaryFile)
	if err != nil {
		return err
	}
	defer sf.Close()

	summary := fmt.Sprintf(`FASTQ拆分报告
生成时间: %s
Excel文件: %s
输出目录: %s

样品统计:
========
总样品数: %d
使用的线程数: %d

质量过滤:
========
最低质量值: %d
最小合并长度: %d

输出文件:
========
报告文件: %s
汇总文件: %s
每个样品的拆分结果在各自子目录中

处理完成!
`,
		time.Now().Format("2006-01-02 15:04:05"),
		s.config.ExcelFile,
		s.config.OutputDir,
		len(s.samples),
		s.config.Threads,
		s.config.Quality,
		s.config.MergeLen,
		reportFile,
		summaryFile,
	)

	sf.WriteString(summary)

	fmt.Printf("  报告已生成: %s\n", reportFile)
	fmt.Printf("  汇总已生成: %s\n", summaryFile)

	return nil
}

// 读取Mmap数据 - 修复版
func readMmapData(filename string) ([]byte, error) {
	// 方法1: 直接读取整个文件到内存
	return os.ReadFile(filename)
}

// 批量读取FASTQ记录
func readFastqRecords(filename string, processor func(record []byte)) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var record bytes.Buffer
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()

		if lineCount == 0 {
			// 新记录开始
			if record.Len() > 0 {
				processor(record.Bytes())
				record.Reset()
			}
		}

		record.Write(line)
		record.WriteByte('\n')
		lineCount = (lineCount + 1) % 4
	}

	// 处理最后一个记录
	if record.Len() > 0 {
		processor(record.Bytes())
	}

	return scanner.Err()
}

// 使用mmap的高级方法（如果需要）
func getMmapData(mmapReader interface{}) ([]byte, error) {
	// 使用反射访问未导出字段
	rv := reflect.ValueOf(mmapReader).Elem()
	dataField := rv.FieldByName("data")

	if dataField.IsValid() && dataField.Kind() == reflect.Slice {
		// 获取切片头
		sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(dataField.UnsafeAddr()))

		// 创建切片
		length := dataField.Len()
		data := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
			Data: sliceHeader.Data,
			Len:  length,
			Cap:  length,
		}))

		return data, nil
	}

	return nil, fmt.Errorf("无法访问mmap数据")
}
