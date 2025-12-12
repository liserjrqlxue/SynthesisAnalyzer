package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
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
	if err := splitter.RunEnhanced(); err != nil {
		log.Fatalf("处理失败: %v", err)
	}

	fmt.Println("处理完成!")
}

// 主运行流程
func (s *RegexpSplitter) RunEnhanced() error {
	startTime := time.Now()
	fmt.Println("=== FASTQ正则表达式拆分程序（增强版）开始运行 ===")

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
	if err := s.splitExtractTargetOnlyEnhanced(); err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}

	// 步骤7: 生成报告
	fmt.Println("\n步骤7: 生成报告...")
	if err := s.generateEnhancedReport(); err != nil {
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

// 读取Mmap数据 - 修复版
func readMmapData(filename string) ([]byte, error) {
	// 方法1: 直接读取整个文件到内存
	return os.ReadFile(filename)
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
