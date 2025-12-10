package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/tealeg/xlsx"
	"golang.org/x/exp/mmap"
)

// 配置结构
type Config struct {
	ExcelFile    string
	OutputDir    string
	Threads      int
	BarcodeStart int
	BarcodeEnd   int
	Mismatch     int
	Quality      int
	MergeLen     int
}

// 样品信息
type SampleInfo struct {
	Name          string
	TargetSeq     string
	SynthesisSeq  string
	PostTargetSeq string
	R1Path        string
	R2Path        string
	BarcodeStart  string
	BarcodeEnd    string
	MergedFile    string
	OutputPath    string
}

// 拆分处理器
type SplitProcessor struct {
	config      *Config
	samples     []*SampleInfo
	sampleMap   map[string]*SampleInfo
	fileMap     map[string][]*SampleInfo // merged文件 -> 样品列表
	bloomFilter *bloom.BloomFilter
	barcodeIdx  map[string]*SampleInfo
	mu          sync.RWMutex
}

// 新建处理器
func NewSplitProcessor(config *Config) *SplitProcessor {
	return &SplitProcessor{
		config:     config,
		samples:    []*SampleInfo{},
		sampleMap:  make(map[string]*SampleInfo),
		fileMap:    make(map[string][]*SampleInfo),
		barcodeIdx: make(map[string]*SampleInfo),
	}
}

func main() {
	// 检查参数
	if len(os.Args) < 2 {
		fmt.Println("用法: fastq_splitter <Excel文件> [输出目录]")
		fmt.Println("示例: fastq_splitter samples.xlsx ./output")
		os.Exit(1)
	}

	excelFile := os.Args[1]
	outputDir := "./output"
	if len(os.Args) > 2 {
		outputDir = os.Args[2]
	}

	// 创建配置
	config := &Config{
		ExcelFile:    excelFile,
		OutputDir:    outputDir,
		Threads:      runtime.NumCPU(),
		BarcodeStart: 20,
		BarcodeEnd:   20,
		Mismatch:     2,
		Quality:      20,
		MergeLen:     280,
	}

	// 创建处理器
	processor := NewSplitProcessor(config)

	// 运行处理流程
	if err := processor.Run(); err != nil {
		log.Fatalf("处理失败: %v", err)
	}

	fmt.Println("处理完成!")
}

// 主运行流程
func (p *SplitProcessor) Run() error {
	startTime := time.Now()
	fmt.Println("=== FASTQ拆分程序开始运行 ===")

	// 步骤1: 创建输出目录
	if err := p.createOutputDir(); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 步骤2: 读取Excel文件
	if err := p.readExcel(); err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}

	// 步骤3: 检查重复样品名称
	if err := p.checkDuplicates(); err != nil {
		return err
	}

	// 步骤4: 合并PE reads
	fmt.Println("\n步骤4: 合并PE reads...")
	if err := p.mergeReads(); err != nil {
		return fmt.Errorf("合并reads失败: %v", err)
	}

	// 步骤5: 构建barcode索引
	fmt.Println("\n步骤5: 构建barcode索引...")
	if err := p.buildBarcodeIndex(); err != nil {
		return fmt.Errorf("构建barcode索引失败: %v", err)
	}

	// 步骤6: 拆分reads
	fmt.Println("\n步骤6: 拆分reads...")
	if err := p.splitReads(); err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}

	// 步骤7: 生成报告
	fmt.Println("\n步骤7: 生成报告...")
	if err := p.generateReport(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	fmt.Printf("\n=== 全部处理完成! 总耗时: %v ===\n", time.Since(startTime))
	return nil
}

// 创建输出目录
func (p *SplitProcessor) createOutputDir() error {
	if err := os.MkdirAll(p.config.OutputDir, 0755); err != nil {
		return err
	}

	// 创建日志目录
	logDir := filepath.Join(p.config.OutputDir, "logs")
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
func (p *SplitProcessor) readExcel() error {
	fmt.Printf("步骤1: 读取Excel文件: %s\n", p.config.ExcelFile)

	xlFile, err := xlsx.OpenFile(p.config.ExcelFile)
	if err != nil {
		return fmt.Errorf("无法打开Excel文件: %v", err)
	}

	if len(xlFile.Sheets) == 0 {
		return fmt.Errorf("Excel文件中没有工作表")
	}

	sheet := xlFile.Sheets[0]
	fmt.Printf("工作表: %s, 行数: %d\n", sheet.Name, sheet.MaxRow)

	// 查找表头
	// headerRow := 0
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
			TargetSeq:     sheet.Cell(row, headerMap["靶标序列"]).Value,
			SynthesisSeq:  sheet.Cell(row, headerMap["合成序列"]).Value,
			PostTargetSeq: sheet.Cell(row, headerMap["后靶标"]).Value,
			R1Path:        sheet.Cell(row, headerMap["路径-R1"]).Value,
			R2Path:        sheet.Cell(row, headerMap["路径-R2"]).Value,
		}

		// 检查必需字段
		if sample.Name == "" {
			continue // 跳过空行
		}

		// 构建输出路径
		sample.OutputPath = filepath.Join(p.config.OutputDir, sample.Name)
		if err := os.MkdirAll(sample.OutputPath, 0755); err != nil {
			return fmt.Errorf("创建样品目录失败: %v", err)
		}

		p.samples = append(p.samples, sample)
		p.sampleMap[sample.Name] = sample

		fmt.Printf("  读取样品: %s, R1: %s, R2: %s\n",
			sample.Name, filepath.Base(sample.R1Path), filepath.Base(sample.R2Path))
	}

	fmt.Printf("  成功读取 %d 个样品\n", len(p.samples))
	return nil
}

// 检查重复样品名称
func (p *SplitProcessor) checkDuplicates() error {
	fmt.Println("\n步骤2: 检查重复样品名称...")

	seen := make(map[string]bool)
	duplicates := []string{}

	for _, sample := range p.samples {
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
func (p *SplitProcessor) mergeReads() error {
	// 去重R1/R2文件对
	filePairs := make(map[string][]*SampleInfo) // key: "R1|R2" -> 样品列表

	for _, sample := range p.samples {
		key := sample.R1Path + "|" + sample.R2Path
		filePairs[key] = append(filePairs[key], sample)
	}

	fmt.Printf("  需要合并 %d 个唯一的R1/R2文件对\n", len(filePairs))

	// 并行合并
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.config.Threads/2) // 限制并发数
	results := make(chan struct {
		key    string
		output string
		err    error
	}, len(filePairs))

	for key, samples := range filePairs {
		wg.Add(1)
		sem <- struct{}{}

		go func(k string, samps []*SampleInfo) {
			defer func() {
				<-sem
				wg.Done()
			}()

			// 生成合并后的文件名
			mergedFile := p.getMergedFileName(samps[0])
			output, err := p.runFastp(samps[0].R1Path, samps[0].R2Path, mergedFile)

			results <- struct {
				key    string
				output string
				err    error
			}{
				key:    k,
				output: output,
				err:    err,
			}
		}(key, samples)
	}

	// 等待所有合并完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	for result := range results {
		if result.err != nil {
			log.Printf("合并失败 (R1: %s): %v", result.key, result.err)
			continue
		}

		// 更新对应样品的MergedFile字段
		samples := filePairs[result.key]
		for _, sample := range samples {
			sample.MergedFile = result.output
			p.fileMap[result.output] = append(p.fileMap[result.output], sample)
		}

		fmt.Printf("  合并完成: %s -> %s (%d个样品)\n",
			result.key, filepath.Base(result.output), len(samples))
	}

	// 检查是否有样品合并失败
	for _, sample := range p.samples {
		if sample.MergedFile == "" {
			return fmt.Errorf("样品 %s 合并失败，请检查输入文件", sample.Name)
		}
	}

	return nil
}

// 生成合并文件名
func (p *SplitProcessor) getMergedFileName(sample *SampleInfo) string {
	// 基于R1和R2路径生成唯一的文件名
	base1 := filepath.Base(sample.R1Path)
	base2 := filepath.Base(sample.R2Path)

	// 去掉扩展名
	base1 = strings.TrimSuffix(base1, filepath.Ext(base1))
	base2 = strings.TrimSuffix(base2, filepath.Ext(base2))

	// 合并目录
	mergedDir := filepath.Join(p.config.OutputDir, "merged")
	os.MkdirAll(mergedDir, 0755)

	// 使用第一个样品的名称作为文件名
	filename := fmt.Sprintf("%s_merged.fastq", sample.Name)
	return filepath.Join(mergedDir, filename)
}

// 运行fastp进行合并
func (p *SplitProcessor) runFastp(r1Path, r2Path, outputFile string) (string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(r1Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R1文件不存在: %s", r1Path)
	}
	if _, err := os.Stat(r2Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R2文件不存在: %s", r2Path)
	}

	// 临时文件
	tempDir := filepath.Join(p.config.OutputDir, "temp")
	os.MkdirAll(tempDir, 0755)

	// fastp命令
	cmd := exec.Command("fastp",
		"--in1", r1Path,
		"--in2", r2Path,
		"--merged_out", outputFile,
		"--out1", "/dev/null",
		"--out2", "/dev/null",
		"--unpaired1", "/dev/null",
		"--unpaired2", "/dev/null",
		"--thread", "4",
		"--merge",
		"--overlap_len_require", "30",
		"--overlap_diff_limit", "5",
		"--qualified_quality_phred", fmt.Sprintf("%d", p.config.Quality),
		"--length_required", fmt.Sprintf("%d", p.config.MergeLen-50),
		"--html", filepath.Join(tempDir, fmt.Sprintf("%s_fastp.html", filepath.Base(outputFile))),
		"--json", filepath.Join(tempDir, fmt.Sprintf("%s_fastp.json", filepath.Base(outputFile))),
	)

	// 执行命令
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("fastp执行失败: %v\n%s", err, stderr.String())
	}

	// 检查输出文件
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		return "", fmt.Errorf("合并后文件未生成: %s", outputFile)
	}

	return outputFile, nil
}

// 构建barcode索引
func (p *SplitProcessor) buildBarcodeIndex() error {
	// 创建Bloom过滤器
	p.bloomFilter = bloom.NewWithEstimates(
		uint(len(p.samples)*2), // 估计元素数量
		0.01,                   // 误报率
	)

	// 为每个样品提取barcode
	for _, sample := range p.samples {
		// 合成序列 = 靶标序列 + 合成序列 + 后靶标
		synthesisSeq := sample.TargetSeq + sample.SynthesisSeq + sample.PostTargetSeq

		// 提取barcode（从合成序列的开头和结尾）
		if len(sample.TargetSeq) >= p.config.BarcodeStart {
			sample.BarcodeStart = sample.TargetSeq[:p.config.BarcodeStart]
		} else {
			sample.BarcodeStart = sample.TargetSeq
		}

		if len(sample.PostTargetSeq) >= p.config.BarcodeEnd {
			sample.BarcodeEnd = sample.PostTargetSeq[:p.config.BarcodeEnd]
		} else {
			sample.BarcodeEnd = sample.PostTargetSeq
		}

		// 添加到Bloom过滤器
		key := sample.BarcodeStart + "|" + sample.BarcodeEnd
		p.bloomFilter.Add([]byte(key))

		// 添加到索引
		p.barcodeIdx[key] = sample

		fmt.Printf("  样品 %s: barcode=[%s...%s], 序列长度=%d\n",
			sample.Name,
			sample.BarcodeStart,
			sample.BarcodeEnd,
			len(synthesisSeq))
	}

	return nil
}

// 拆分reads
func (p *SplitProcessor) splitReads() error {
	totalReads := 0
	totalSamples := 0

	// 为每个merged文件启动一个拆分任务
	for mergedFile, samples := range p.fileMap {
		fmt.Printf("\n  处理合并文件: %s (%d个样品)\n",
			filepath.Base(mergedFile), len(samples))

		// 并行处理每个样品
		var wg sync.WaitGroup
		results := make(chan map[string]int, len(samples))

		// 为每个样品创建输出文件
		sampleFiles := make(map[string]*os.File)
		for _, sample := range samples {
			outputFile := filepath.Join(sample.OutputPath, "split_reads.fastq")
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("创建输出文件失败: %v", err)
			}
			sampleFiles[sample.Name] = f
		}

		// 读取合并文件
		mmapReader, err := mmap.Open(mergedFile)
		if err != nil {
			return fmt.Errorf("内存映射失败: %v", err)
		}

		data := mmapReader.Data()
		chunkSize := 64 * 1024 * 1024 // 64MB chunks
		numChunks := (len(data) + chunkSize - 1) / chunkSize

		fmt.Printf("    文件大小: %.2f MB, 数据块数: %d\n",
			float64(len(data))/1024/1024, numChunks)

		// 处理每个数据块
		for chunk := 0; chunk < numChunks; chunk++ {
			wg.Add(1)
			go func(chunkIdx int) {
				defer wg.Done()

				start := chunkIdx * chunkSize
				end := start + chunkSize
				if end > len(data) {
					end = len(data)
				}

				chunkData := data[start:end]
				stats := p.processChunk(chunkData, samples, sampleFiles)

				results <- stats
			}(chunk)
		}

		// 等待所有块处理完成
		go func() {
			wg.Wait()
			close(results)
		}()

		// 收集统计信息
		chunkStats := make(map[string]int)
		for stats := range results {
			for sampleName, count := range stats {
				chunkStats[sampleName] += count
				totalReads += count
			}
		}

		// 关闭文件
		for _, f := range sampleFiles {
			f.Close()
		}

		// 输出统计
		for sampleName, count := range chunkStats {
			fmt.Printf("    样品 %s: %d reads\n", sampleName, count)
			totalSamples++
		}

		mmapReader.Close()
	}

	fmt.Printf("\n  总计: %d 个样品, %d 条reads\n", totalSamples, totalReads)
	return nil
}

// 处理数据块
func (p *SplitProcessor) processChunk(data []byte, samples []*SampleInfo, sampleFiles map[string]*os.File) map[string]int {
	stats := make(map[string]int)

	i := 0
	for i < len(data) {
		// 查找FASTQ记录开始
		if data[i] != '@' {
			// 跳过到下一行
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				i++ // 跳过换行符
			}
			continue
		}

		// 找到记录开始
		recordStart := i

		// 找到记录结束（4行后）
		linesFound := 0
		for linesFound < 4 && i < len(data) {
			// 查找行尾
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				i++ // 跳过换行符
				linesFound++
			}
		}

		if linesFound < 4 {
			break // 不完整的记录
		}

		// 提取记录
		record := data[recordStart:i]

		// 解析序列（第二行）
		lines := bytes.SplitN(record, []byte{'\n'}, 4)
		if len(lines) < 2 {
			continue
		}

		sequence := lines[1]

		// 提取barcode
		if len(sequence) >= p.config.BarcodeStart+p.config.BarcodeEnd {
			startBarcode := string(sequence[:p.config.BarcodeStart])
			endBarcode := string(sequence[len(sequence)-p.config.BarcodeEnd:])

			// 使用Bloom过滤器快速检查
			key := startBarcode + "|" + endBarcode
			if !p.bloomFilter.Test([]byte(key)) {
				// 尝试反向互补
				rcKey := reverseComplement(startBarcode) + "|" + reverseComplement(endBarcode)
				if !p.bloomFilter.Test([]byte(rcKey)) {
					continue // 不匹配任何barcode
				} else {
					key = rcKey
				}
			}

			// 查找匹配的样品
			sample, ok := p.barcodeIdx[key]
			if !ok {
				// 如果没有完全匹配，尝试模糊匹配
				sample = p.fuzzyMatchBarcode(startBarcode, endBarcode, samples)
			}

			if sample != nil {
				// 写入到对应文件
				if f, ok := sampleFiles[sample.Name]; ok {
					f.Write(record)
					stats[sample.Name]++
				}
			}
		}
	}

	return stats
}

// 模糊匹配barcode
func (p *SplitProcessor) fuzzyMatchBarcode(start, end string, samples []*SampleInfo) *SampleInfo {
	var bestMatch *SampleInfo
	bestScore := -1

	for _, sample := range samples {
		score := 0

		// 比较start barcode
		startLen := min(len(start), len(sample.BarcodeStart))
		for i := 0; i < startLen; i++ {
			if start[i] == sample.BarcodeStart[i] {
				score++
			}
		}

		// 比较end barcode
		endLen := min(len(end), len(sample.BarcodeEnd))
		for i := 0; i < endLen; i++ {
			if end[i] == sample.BarcodeEnd[i] {
				score++
			}
		}

		// 计算总匹配率
		totalLen := startLen + endLen
		matchRate := float64(score) / float64(totalLen)

		// 如果匹配率足够高
		if matchRate >= 0.9 && score > bestScore { // 允许10%的错误
			bestScore = score
			bestMatch = sample
		}
	}

	return bestMatch
}

// 反向互补
func reverseComplement(seq string) string {
	var rc strings.Builder
	rc.Grow(len(seq))

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

// 生成报告
func (p *SplitProcessor) generateReport() error {
	reportFile := filepath.Join(p.config.OutputDir, "split_report.csv")
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
		"靶标序列长度",
		"合成序列长度",
		"后靶标长度",
		"总序列长度",
		"barcode起始",
		"barcode结束",
		"R1文件",
		"R2文件",
		"合并文件",
		"输出目录",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range p.samples {
		// 统计输出文件中的reads数量
		outputFile := filepath.Join(sample.OutputPath, "split_reads.fastq")
		readCount := 0
		if _, err := os.Stat(outputFile); err == nil {
			readCount = countFastqRecords(outputFile)
		}

		record := []string{
			sample.Name,
			fmt.Sprintf("%d", len(sample.TargetSeq)),
			fmt.Sprintf("%d", len(sample.SynthesisSeq)),
			fmt.Sprintf("%d", len(sample.PostTargetSeq)),
			fmt.Sprintf("%d", len(sample.TargetSeq)+len(sample.SynthesisSeq)+len(sample.PostTargetSeq)),
			sample.BarcodeStart,
			sample.BarcodeEnd,
			filepath.Base(sample.R1Path),
			filepath.Base(sample.R2Path),
			filepath.Base(sample.MergedFile),
			sample.OutputPath,
			fmt.Sprintf("%d", readCount),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	// 生成汇总统计
	summaryFile := filepath.Join(p.config.OutputDir, "summary.txt")
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

barcode配置:
==========
起始barcode长度: %d
结束barcode长度: %d
允许错配数: %d

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
		p.config.ExcelFile,
		p.config.OutputDir,
		len(p.samples),
		p.config.Threads,
		p.config.BarcodeStart,
		p.config.BarcodeEnd,
		p.config.Mismatch,
		p.config.Quality,
		p.config.MergeLen,
		reportFile,
		summaryFile,
	)

	sf.WriteString(summary)

	fmt.Printf("  报告已生成: %s\n", reportFile)
	fmt.Printf("  汇总已生成: %s\n", summaryFile)

	return nil
}

// 统计FASTQ记录数
func countFastqRecords(filename string) int {
	file, err := os.Open(filename)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum%4 == 1 && len(scanner.Bytes()) > 0 && scanner.Bytes()[0] == '@' {
			count++
		}
	}

	return count
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
