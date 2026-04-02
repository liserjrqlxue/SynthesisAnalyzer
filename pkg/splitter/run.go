package splitter

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"SynthesisAnalyzer/pkg/cfg"

	// "compress/gzip"
	gzip "github.com/klauspost/pgzip"
)

// 创建比对分析器
func NewAlignmentAnalyzer(config *cfg.AlignmentConfig, samples []*cfg.Sample, outputDir string) *AlignmentAnalyzer {
	return &AlignmentAnalyzer{
		config:    config,
		samples:   samples,
		outputDir: outputDir,
		stats:     &AlignmentStats{},
	}
}

// 计算测序时间
func (s *EnhancedSplitter) calculateSequencingTime() error {
	fastqDir := s.config.FastqDir

	// 判断模式
	if strings.Contains(fastqDir, "/G99/") {
		// G99模式：解析version.json
		versionFile := filepath.Join(fastqDir, "version.json")
		content, err := os.ReadFile(versionFile)
		if err != nil {
			return fmt.Errorf("failed to read version.json: %v", err)
		}

		// 简单解析DateTime字段
		dateTimeRegex := regexp.MustCompile(`"DateTime":\s*"([^"]+)"`)
		matches := dateTimeRegex.FindSubmatch(content)
		if len(matches) < 2 {
			return fmt.Errorf("DateTime field not found in version.json")
		}

		dateTimeStr := string(matches[1])
		// 解析时间格式：2026-03-13 04:17:38
		t, err := time.Parse("2006-01-02 15:04:05", dateTimeStr)
		if err != nil {
			return fmt.Errorf("failed to parse DateTime: %v", err)
		}

		s.sequencingTime = t.Format("2006.01.02")
	} else if strings.Contains(fastqDir, "/novo-medical-customer-tj/") {
		// Novo模式：使用最后的目录的头部如20260311
		parts := strings.Split(fastqDir, "/")
		if len(parts) < 2 {
			return fmt.Errorf("invalid fastq directory path")
		}

		// 找到包含日期的目录名
		for i := len(parts) - 1; i >= 0; i-- {
			if len(parts[i]) >= 8 {
				// 提取前8个字符作为日期
				datePart := parts[i][:8]
				if matched, _ := regexp.MatchString(`^\d{8}$`, datePart); matched {
					s.sequencingTime = datePart
					return nil
				}
			}
		}

		return fmt.Errorf("date not found in novo directory path")
	}

	return nil
}

// 扩展的主运行函数，包含比对步骤
func (s *EnhancedSplitter) RunAll() error {
	fmt.Println("=== FASTQ拆分与比对分析完整流程 ===")

	// 计算测序时间
	if err := s.calculateSequencingTime(); err != nil {
		fmt.Printf("警告: 计算测序时间失败: %v\n", err)
	}

	// 阶段1: 拆分
	fmt.Println("\n--- 阶段1: FASTQ拆分 ---")
	if err := s.RunSplitter(); err != nil {
		return fmt.Errorf("拆分阶段失败: %v", err)
	}

	// 阶段2: 比对分析（如果启用）
	fmt.Println("\n--- 阶段2: 序列比对分析 ---")
	if err := s.RunAlignment(); err != nil {
		return fmt.Errorf("比对分析失败: %v", err)
	}

	// 生成最终汇总
	if err := s.generateFinalSummary(); err != nil {
		return fmt.Errorf("生成最终汇总失败: %v", err)
	}

	fmt.Println("\n=== 所有处理完成! ===")
	return nil
}

// 扩展的主运行函数，包含比对步骤
func (s *EnhancedSplitter) RunAlignment() error {
	// 阶段2: 比对分析
	fmt.Println("\n--- 阶段2: 序列比对分析 ---")
	// 创建比对分析器
	analyzer := NewAlignmentAnalyzer(&s.config.Alignment, s.samples, s.config.OutputDir)

	// 步骤1: 创建参考序列
	if err := analyzer.createReferenceFiles(); err != nil {
		return fmt.Errorf("创建参考序列失败: %v", err)
	}

	// 步骤2: 运行比对
	if err := analyzer.runAlignment(); err != nil {
		return fmt.Errorf("比对失败: %v", err)
	}

	// 步骤3: 生成报告
	if err := analyzer.generateAlignmentReport(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	// 步骤4: 生成质量控制报告
	if err := analyzer.generateQCReport(); err != nil {
		fmt.Printf("警告: 生成QC报告失败: %v\n", err)
	}

	// 步骤5: 执行交叉污染检测（如果启用）
	if s.config.ContaminationDetection {
		if err := analyzer.performContaminationDetection(); err != nil {
			fmt.Printf("警告: 交叉污染检测失败: %v\n", err)
		}
	}
	return nil
}

// 主运行流程
func (s *EnhancedSplitter) RunSplitter() error {
	// 初始化统计
	s.stats = &SplitStats{
		startTime: time.Now(),
	}
	defer func() {
		s.stats.endTime = time.Now()
		fmt.Printf("\n=== 全部处理完成! 总耗时: %v ===\n", s.stats.endTime.Sub(s.stats.startTime))
		fmt.Printf("处理统计: %d 个文件, %d 个样品, %d 条reads, %d 条匹配 (%.1f%%)\n",
			s.stats.totalFiles, s.stats.totalSamples, s.stats.totalReads, s.stats.totalMatched,
			float64(s.stats.totalMatched)/float64(s.stats.totalReads)*100)
	}()

	fmt.Println("=== 增强版FASTQ拆分程序开始运行 ===")
	fmt.Printf("配置: 线程数=%d, 反向互补=%v, 跳过已存在=%v\n",
		s.config.Threads, s.config.UseRC, s.config.SkipExisting)

	// 步骤1: 读取Excel文件
	fmt.Printf("\n步骤1: 读取Excel文件: %s\n", s.config.ExcelFile)
	if err := s.loadSamplesFromExcel(); err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}

	// 步骤2: 检查重复样品名称
	fmt.Println("\n步骤2: 检查重复样品名称...")
	if err := s.checkDuplicates(); err != nil {
		return err
	}

	// 步骤3: 创建输出目录
	fmt.Printf("\n步骤3: 创建输出目录: %s\n", s.config.OutputDir)
	if err := s.createOutputDir(); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 步骤4: 合并PE reads并建立文件映射
	fmt.Println("\n步骤4: 合并PE reads并建立文件映射...")
	if err := s.mergeAndMapFiles(); err != nil {
		return fmt.Errorf("合并reads失败: %v", err)
	}

	// 步骤5: 为每个合并文件构建匹配器
	fmt.Println("\n步骤5: 为每个合并文件构建匹配器...")
	if err := s.buildFileMatchers(); err != nil {
		return fmt.Errorf("构建索引失败: %v", err)
	}

	// 检查全局完成标签
	runDoneFile := filepath.Join(s.config.OutputDir, "run.done")
	if _, err := os.Stat(runDoneFile); err == nil {
		fmt.Println("  检测到run.done文件，后续步骤已跳过，如需重新运行，请删除[", runDoneFile, "]后重跑")
		return nil
	}

	// 步骤6: 独立处理每个合并文件
	fmt.Println("\n步骤6: 独立处理每个合并文件...")
	if err := s.processEachFileSeparately(); err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}

	// 步骤7: 生成报告
	fmt.Println("\n步骤7: 生成报告...")
	if err := s.generateReports(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	// 8. 可选：调试模式（输出未匹配的序列）
	if os.Getenv("DEBUG") == "1" {
		for mergedFile := range s.fileMap {
			s.debugUnmatched(mergedFile, s.config.OutputDir)
			break // 只调试第一个文件
		}
	}

	// 创建全局完成标签
	content := fmt.Sprintf("Created: %s\nTotalFiles: %d\nTotalSamples: %d\nTotalReads: %d\nTotalMatched: %d\nMatchingRate: %.1f%%\n",
		time.Now().Format(time.RFC3339),
		s.stats.totalFiles, s.stats.totalSamples, s.stats.totalReads, s.stats.totalMatched,
		float64(s.stats.totalMatched)/float64(s.stats.totalReads)*100)
	if err := os.WriteFile(runDoneFile, []byte(content), 0644); err != nil {
		fmt.Printf("  警告: 创建全局完成标签失败: %v\n", err)
	}

	return nil
}

// 创建输出目录
func (s *EnhancedSplitter) createOutputDir() error {
	if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
		return err
	}

	// 创建样品目录
	samplesDir := filepath.Join(s.config.OutputDir, "samples")
	if err := os.MkdirAll(samplesDir, 0755); err != nil {
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

	for _, sample := range s.samples {
		// 创建输出目录
		sample.OutputDir = filepath.Join(s.config.OutputDir, "samples", sample.Name)
		if err := os.MkdirAll(sample.OutputDir, 0755); err != nil {
			return fmt.Errorf("创建样品目录失败: %v", err)
		}
	}

	return nil
}

// 读取Excel文件
func (s *EnhancedSplitter) loadSamplesFromExcel() error {
	samples, err := s.config.LoadInputExcel()
	if err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}
	s.samples = samples

	for _, sample := range s.samples {

		// 创建输出目录
		sample.OutputDir = filepath.Join(s.config.OutputDir, "samples", sample.Name)
		if err := os.MkdirAll(sample.OutputDir, 0755); err != nil {
			return fmt.Errorf("创建样品目录失败: %v", err)
		}
	}

	fmt.Printf("  成功读取 %d 个样品\n", len(s.samples))
	return nil
}

// 检查重复样品名称
func (s *EnhancedSplitter) checkDuplicates() error {

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

// 使用goroutine管道处理
func (s *EnhancedSplitter) splitReadsPipeline() error {
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
			wg.Go(func() {
				for record := range recordChan {
					sequence, found := extractSequence(record)
					if found {
						sample, direction := s.matchSequence(sequence)
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
			})
		}

		// 启动写入goroutine
		writers := make(map[string]*gzip.Writer)
		writerFiles := make(map[string]*os.File)

		for _, sample := range samples {
			outputFile := filepath.Join(sample.OutputDir, "split_reads.fastq.gz")
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
func (s *EnhancedSplitter) readRecordsToChannel(filename string, recordChan chan<- []byte) error {

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
func (s *EnhancedSplitter) matchSequence(sequence []byte) (*cfg.Sample, string) {
	seqStr := string(sequence)
	// 完全匹配
	sample, direction := s.exactMatch(seqStr)
	if sample == nil {
		s.statsMutex.Lock()
		s.stats.totalFailed++
		s.statsMutex.Unlock()
		return nil, ""
	}
	return sample, direction
}
