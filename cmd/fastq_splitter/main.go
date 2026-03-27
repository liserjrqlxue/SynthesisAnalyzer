package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// "compress/gzip"

	gzip "github.com/klauspost/pgzip"
	"github.com/tealeg/xlsx"
)

var (
	excelFile = flag.String(
		"i",
		"",
		"<Excel文件>",
	)
	outputDir = flag.String(
		"o",
		"",
		"[输出目录]",
	)
	fastqDir = flag.String(
		"fq",
		"",
		"[Fastq目录]",
	)
	suffixCol = flag.String(
		"suffix-col",
		"",
		"可选参数：样品名称后缀列，若指定则将该列值拼接到样品名称后",
	)
	overlap = flag.Int(
		"m",
		30,
		"the minimum length to detect overlapped region of PE reads. This will affect overlap analysis based PE merge, adapter trimming and correction. 3",
	)
)

func main() {
	flag.Parse()
	if *excelFile == "" {
		flag.Usage()
		log.Fatalln("-i required!")
	}

	// 处理输出目录
	output := *outputDir
	if output == "" {
		// 如果-o未定义，使用-i 切掉".xlsx"作为输出目录
		baseName := filepath.Base(*excelFile)
		if before, ok := strings.CutSuffix(baseName, ".xlsx"); ok {
			output = before
		} else {
			output = baseName
		}
	}

	// 处理fastq目录
	fastq := *fastqDir
	if fastq == "" {
		// 如果-fq未定义，查找-i对应目录内的*path.txt(兼容大写字符)
		excelDir := filepath.Dir(*excelFile)
		files, err := filepath.Glob(filepath.Join(excelDir, "*[Pp][Aa][Tt][Hh].txt"))
		if err != nil || len(files) == 0 {
			log.Fatalln("No path.txt file found in the Excel directory")
		}

		// 读取path.txt文件内容
		pathFile := files[0]
		content, err := os.ReadFile(pathFile)
		if err != nil {
			log.Fatalf("Failed to read path.txt: %v", err)
		}

		fqBatch := strings.TrimSpace(string(content))
		if fqBatch == "" {
			log.Fatalln("Empty content in path.txt")
		}

		// 判断模式
		if matched, _ := regexp.MatchString(`^FT\d+$`, fqBatch); matched {
			// G99模式
			fastq = fmt.Sprintf("/data2/wangyaoshen/Sequencing_data/G99/R21007100240139/%s/L01", fqBatch)
		} else if strings.HasPrefix(fqBatch, "oss://novo-medical-customer-tj/") {
			// Novo模式
			parts := strings.Split(fqBatch, "/")
			if len(parts) < 4 {
				log.Fatalln("Invalid Novo path format")
			}
			// 提取最后一个目录
			lastDir := parts[len(parts)-1]
			// 提取CYB编号 (parts[3] after splitting "oss://novo-medical-customer-tj/CYB24030020/...")
			cyb := parts[3]
			fastq = fmt.Sprintf("/data2/wangyaoshen/novo-medical-customer-tj/%s/%s/Rawdata", cyb, lastDir)
		} else {
			log.Fatalln("Unsupported fqBatch format")
		}
	}

	// 创建配置
	config := &Config{
		ExcelFile:        *excelFile,
		OutputDir:        output,
		FastqDir:         fastq,
		Threads:          runtime.NumCPU(),
		SampleNameSuffix: *suffixCol,
		SearchWindow:     200, // 从头/尾搜索50bp
		Quality:          20,
		MergeLen:         80,

		UseRC:         true, // 启用反向互补匹配
		SkipExisting:  true, // 默认跳过已存在文件
		Compression:   true, // 默认启用压缩
		CompressLevel: 6,    // 默认压缩级别
		CleanupTemp:   true, // 不保留临时文件

		AllowMismatch:  0,             // 允许2个错配
		MatchThreshold: 30,            // 匹配分数阈值
		OutputMode:     "target-only", // 只输出靶标间序列

		Alignment: AlignmentConfig{
			UseMinimap2:    true,
			AlignerThreads: 8,
			MapQThreshold:  10,
			MinIdentity:    0.90,
			SkipAlignment:  false,
			KeepSamFiles:   false,
			AnalysisOnly:   false,
		},

		OverlapLenRequire: *overlap,
	}

	// 处理可选参数
	for i := 4; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--skip-alignment":
			config.Alignment.SkipAlignment = true
		case "--analysis-only":
			config.Alignment.AnalysisOnly = true
		case "--keep-bam":
			config.Alignment.KeepSamFiles = true
		case "--threads":
			if i+1 < len(os.Args) {
				threads, err := strconv.Atoi(os.Args[i+1])
				if err == nil && threads > 0 {
					config.Threads = threads
				}
				i++
			}
		}
	}

	// 创建处理器
	splitter := NewEnhancedSplitter(config)

	// 运行处理流程
	if err := splitter.RunWithAlignment(); err != nil {
		fmt.Printf("处理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("处理完成!")
}

func printUsage() {
	fmt.Println(`FASTQ拆分与比对分析系统 v2.0

用法:
  fastq_analyzer <Excel文件> [输出目录] [Fastq目录] [选项]

示例:
  fastq_analyzer samples.xlsx ./results
  fastq_analyzer samples.xlsx ./output ./fastq --skip-alignment --threads 32

选项:
  --skip-alignment    跳过比对步骤（仅拆分）
  --analysis-only    仅分析已有的BAM文件
  --keep-bam         保留BAM文件（默认清理）
  --threads N        设置线程数

输入Excel格式:
  Sheet1必须包含以下列：
    - 样品名称
    - 靶标序列
    - 合成序列
    - 后靶标
    - 路径-R1
    - 路径-R2

依赖软件:
  - fastp: 用于PE reads合并
  - minimap2: 用于序列比对
  - samtools: 用于BAM文件处理
  - R: 用于统计绘图（可选）`)
}

// 创建比对分析器
func NewAlignmentAnalyzer(config *AlignmentConfig, samples []*SampleInfo, outputDir string) *AlignmentAnalyzer {
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
func (s *EnhancedSplitter) RunWithAlignment() error {
	fmt.Println("=== FASTQ拆分与比对分析完整流程 ===")

	// 计算测序时间
	if err := s.calculateSequencingTime(); err != nil {
		fmt.Printf("警告: 计算测序时间失败: %v\n", err)
	}

	// 阶段1: 拆分
	fmt.Println("\n--- 阶段1: FASTQ拆分 ---")
	if err := s.Run(); err != nil {
		return fmt.Errorf("拆分阶段失败: %v", err)
	}

	// 阶段2: 比对分析（如果启用）
	if !s.config.Alignment.SkipAlignment {
		fmt.Println("\n--- 阶段2: 序列比对分析 ---")

		// 创建比对分析器
		analyzer := NewAlignmentAnalyzer(&s.config.Alignment, s.samples, s.config.OutputDir)

		// 步骤1: 创建参考序列
		if err := analyzer.createReferenceFiles(); err != nil {
			return fmt.Errorf("创建参考序列失败: %v", err)
		}

		// 步骤2: 运行比对
		if !s.config.Alignment.AnalysisOnly {
			if err := analyzer.runAlignment(); err != nil {
				return fmt.Errorf("比对失败: %v", err)
			}
		}

		// 步骤3: 生成报告
		if err := analyzer.generateAlignmentReport(); err != nil {
			return fmt.Errorf("生成报告失败: %v", err)
		}

		// 步骤4: 生成质量控制报告
		if err := analyzer.generateQCReport(); err != nil {
			fmt.Printf("警告: 生成QC报告失败: %v\n", err)
		}
	}

	// 生成最终汇总
	if err := s.generateFinalSummary(); err != nil {
		return fmt.Errorf("生成最终汇总失败: %v", err)
	}

	fmt.Println("\n=== 所有处理完成! ===")
	return nil
}

// 主运行流程
func (s *EnhancedSplitter) Run() error {
	startTime := time.Now()
	fmt.Println("=== 增强版FASTQ拆分程序开始运行 ===")
	fmt.Printf("配置: 线程数=%d, 反向互补=%v, 跳过已存在=%v\n",
		s.config.Threads, s.config.UseRC, s.config.SkipExisting)

	// 初始化统计
	s.stats = &SplitStats{
		startTime: startTime,
	}

	// 步骤1: 创建输出目录
	if err := s.createOutputDir(); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 步骤2: 读取Excel文件
	if err := s.loadSamplesFromExcel(); err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}

	// 步骤3: 检查重复样品名称
	if err := s.checkDuplicates(); err != nil {
		return err
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

	s.stats.endTime = time.Now()

	fmt.Printf("\n=== 全部处理完成! 总耗时: %v ===\n", s.stats.endTime.Sub(startTime))
	fmt.Printf("处理统计: %d 个文件, %d 个样品, %d 条reads, %d 条匹配 (%.1f%%)\n",
		s.stats.totalFiles, s.stats.totalSamples, s.stats.totalReads, s.stats.totalMatched,
		float64(s.stats.totalMatched)/float64(s.stats.totalReads)*100)

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

	return nil
}

// 读取Excel文件
func (s *EnhancedSplitter) loadSamplesFromExcel() error {
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

	// 检查后缀列
	var suffixColIndex int = -1
	if s.config.SampleNameSuffix != "" {
		if idx, ok := headerMap[s.config.SampleNameSuffix]; ok {
			suffixColIndex = idx
		} else {
			return fmt.Errorf("未找到指定的后缀列: %s", s.config.SampleNameSuffix)
		}
	}

	// 读取数据行
	seenSampleNames := make(map[string]bool)
	for row := 1; row < sheet.MaxRow; row++ {
		sampleName := sheet.Cell(row, headerMap["样品名称"]).Value
		targetSeq := strings.ToUpper(sheet.Cell(row, headerMap["靶标序列"]).Value)
		synthSeq := strings.ToUpper(sheet.Cell(row, headerMap["合成序列"]).Value)
		postSeq := strings.ToUpper(sheet.Cell(row, headerMap["后靶标"]).Value)
		r1Path := sheet.Cell(row, headerMap["路径-R1"]).Value
		r2Path := sheet.Cell(row, headerMap["路径-R2"]).Value

		// 检查必需字段
		if sampleName == "" {
			continue // 跳过空行
		}

		// 拼接后缀列到样品名称
		if s.config.SampleNameSuffix != "" && suffixColIndex != -1 {
			suffix := strings.TrimSpace(sheet.Cell(row, suffixColIndex).Value)
			if suffix != "" {
				sampleName = sampleName + "." + suffix
			}
		}

		// 检查样品名称是否重复
		if seenSampleNames[sampleName] {
			return fmt.Errorf("第 %d 行样品名称重复: %s", row+1, sampleName)
		}
		seenSampleNames[sampleName] = true

		sample := &SampleInfo{
			Name:          sampleName,
			TargetSeq:     targetSeq,
			SynthesisSeq:  synthSeq,
			PostTargetSeq: postSeq,
			R1Path:        r1Path,
			R2Path:        r2Path,
		}

		// 创建输出目录
		sample.OutputPath = filepath.Join(s.config.OutputDir, "samples", sample.Name)
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
func (s *EnhancedSplitter) checkDuplicates() error {
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
			outputFile := filepath.Join(sample.OutputPath, "split_reads.fastq.gz")
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
func (s *EnhancedSplitter) matchSequence(sequence []byte, samples []*SampleInfo) (*SampleInfo, string) {
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
