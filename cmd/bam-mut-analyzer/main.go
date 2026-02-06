package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
)

// ReadType 表示read的类型
type ReadType int

const (
	ReadTypeMatch              ReadType = iota // 匹配
	ReadTypeInsert                             // 插入
	ReadTypeDelete                             // 缺失
	ReadTypeSubstitution                       // 替换
	ReadTypeInsertDelete                       // 插入+缺失
	ReadTypeInsertSubstitution                 // 插入+替换
	ReadTypeDeleteSubstitution                 // 缺失+替换
	ReadTypeAll                                // 插入+缺失+替换
)

// ReadTypeNames ReadType对应的名称
var ReadTypeNames = map[ReadType]string{
	ReadTypeMatch:              "匹配",
	ReadTypeInsert:             "插入",
	ReadTypeDelete:             "缺失",
	ReadTypeSubstitution:       "替换",
	ReadTypeInsertDelete:       "插入+缺失",
	ReadTypeInsertSubstitution: "插入+替换",
	ReadTypeDeleteSubstitution: "缺失+替换",
	ReadTypeAll:                "插入+缺失+替换",
}

// MutationStats 存储突变统计
type MutationStats struct {
	sync.RWMutex
	PositionMutations    map[string]map[string]int            // sample -> position:mutation -> count
	SampleMutations      map[string]map[string]int            // sample -> mutation -> count
	TotalMutations       map[string]int                       // mutation -> total count
	PositionDetails      map[string]map[string]map[string]int // sample -> position -> mutation -> count
	SampleReadCounts     map[string]int                       // sample -> total reads count
	SampleReadsWithMuts  map[string]int                       // sample -> 包含突变的reads数
	SampleReadTypeCounts map[string]map[ReadType]int          // sample -> read type -> count
	TotalReadCount       int                                  // 所有样本的总reads数
	TotalReadsWithMuts   int                                  // 所有样本的包含突变reads数的和
	TotalReadTypeCounts  map[ReadType]int                     // 所有样本的read type统计
}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	return &MutationStats{
		PositionMutations:    make(map[string]map[string]int),
		SampleMutations:      make(map[string]map[string]int),
		TotalMutations:       make(map[string]int),
		PositionDetails:      make(map[string]map[string]map[string]int),
		SampleReadCounts:     make(map[string]int),
		SampleReadsWithMuts:  make(map[string]int),
		SampleReadTypeCounts: make(map[string]map[ReadType]int),
		TotalReadCount:       0,
		TotalReadsWithMuts:   0,
		TotalReadTypeCounts:  make(map[ReadType]int),
	}
}

// Mutation 表示一个碱基突变
type Mutation struct {
	Position int
	Ref      string
	Alt      string
}

// MDOp 表示MD标签的操作
type MDOp struct {
	Type   byte
	Length int
	Base   byte
	Bases  string
}

// 全局变量，用于存储命令行参数
var (
	inputDir    string
	outputDir   string
	excelFile   string
	sampleOrder []string // 样本顺序列表
)

func init() {
	// 定义命令行参数
	flag.StringVar(&inputDir, "d", "", "输入目录，包含样本子目录")
	flag.StringVar(&outputDir, "o", "", "输出目录, 默认输入目录/mutation_stats")
	flag.StringVar(&excelFile, "i", "", "可选参数：输入Excel文件，包含样本顺序")
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func parseNumber(s string) int {
	num := 0
	for i := 0; i < len(s); i++ {
		num = num*10 + int(s[i]-'0')
	}
	return num
}

// processBAMFile 处理单个BAM文件
func processBAMFile(bamPath, sampleName string, stats *MutationStats) error {
	// 打开BAM文件
	f, err := os.Open(bamPath)
	if err != nil {
		return fmt.Errorf("打开BAM文件失败: %v", err)
	}
	defer f.Close()

	br, err := bam.NewReader(f, 0)
	if err != nil {
		return fmt.Errorf("创建BAM读取器失败: %v", err)
	}
	defer br.Close()

	fmt.Printf("处理样本: %s\n", sampleName)

	var totalReads, readsWithMutations, totalMutations int

	var totalXReads int
	var debugCount int
	var totalXOps int

	// 初始化样本的read type统计
	stats.Lock()
	if _, ok := stats.SampleReadTypeCounts[sampleName]; !ok {
		stats.SampleReadTypeCounts[sampleName] = make(map[ReadType]int)
	}
	stats.Unlock()

	// 遍历所有记录
	for {
		read, err := br.Read()
		if err != nil {
			break // 可能到达文件末尾
		}

		totalReads++

		// 跳过未比对或质量太低的记录
		if read.Flags&sam.Unmapped != 0 {
			continue
		}

		// 分析read类型
		readType := analyzeReadType(read)

		// 更新read type统计
		stats.Lock()
		stats.SampleReadTypeCounts[sampleName][readType]++
		stats.TotalReadTypeCounts[readType]++
		stats.Unlock()

		// 解析突变
		mutations := parseMutationsWithMD(read)

		// 调试：检查是否有X操作
		hasX := debugCigar(read)
		if hasX {
			totalXReads++
		}

		// 统计X操作
		xCount := countXOperations(read)
		if xCount > 0 {
			totalXOps += xCount
			// 如果突变数不等于X操作数，记录差异
			if len(mutations) != xCount {
				// 如果有差异，记录详细信息（仅前几个）
				if debugCount < 3 {
					fmt.Printf("  差异: X操作数=%d, 解析到的突变数=%d\n", xCount, len(mutations))
					fmt.Printf("  CIGAR: %v, POS: %d\n", read.Cigar, read.Pos)
					if mdTag, found := read.Tag([]byte{'M', 'D'}); found {
						fmt.Printf("  MD标签: %s\n", mdTag.String())
					}
					debugCount++
				}
			}
		}

		if len(mutations) > 0 {
			readsWithMutations++
			totalMutations += len(mutations)

			// 更新统计
			stats.Lock()

			// 初始化样本的数据结构
			if _, ok := stats.PositionDetails[sampleName]; !ok {
				stats.PositionDetails[sampleName] = make(map[string]map[string]int)
			}
			if _, ok := stats.SampleMutations[sampleName]; !ok {
				stats.SampleMutations[sampleName] = make(map[string]int)
			}
			if _, ok := stats.PositionMutations[sampleName]; !ok {
				stats.PositionMutations[sampleName] = make(map[string]int)
			}

			for _, mut := range mutations {
				// 创建键
				posKey := fmt.Sprintf("%d", mut.Position)
				mutKey := fmt.Sprintf("%s>%s", mut.Ref, mut.Alt)
				posMutKey := fmt.Sprintf("%s_%s", posKey, mutKey)

				// 更新位置细节
				if _, ok := stats.PositionDetails[sampleName][posKey]; !ok {
					stats.PositionDetails[sampleName][posKey] = make(map[string]int)
				}
				stats.PositionDetails[sampleName][posKey][mutKey]++

				// 更新样本统计
				stats.SampleMutations[sampleName][mutKey]++

				// 更新位置突变统计
				stats.PositionMutations[sampleName][posMutKey]++

				// 更新总统计
				stats.TotalMutations[mutKey]++
			}

			stats.Unlock()
		}
	}

	// 更新reads计数
	stats.Lock()
	stats.SampleReadCounts[sampleName] = totalReads
	stats.SampleReadsWithMuts[sampleName] = readsWithMutations
	stats.TotalReadCount += totalReads
	stats.TotalReadsWithMuts += readsWithMutations
	stats.Unlock()

	// 打印read type统计
	stats.RLock()
	readTypeCounts := stats.SampleReadTypeCounts[sampleName]
	stats.RUnlock()

	fmt.Printf("  总reads数: %d\n", totalReads)
	fmt.Printf("  [DEBUG]有CIGAR X操作的reads数: %d\n", totalXReads)
	fmt.Printf("  包含突变的reads数: %d\n", readsWithMutations)
	fmt.Printf("  总X操作数: %d\n", totalXOps)
	fmt.Printf("  总突变数: %d\n", totalMutations)
	fmt.Printf("  Read类型统计:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := readTypeCounts[rt]
		if count > 0 {
			percentage := float64(count) / float64(totalReads) * 100
			fmt.Printf("    %s: %d (%.2f%%)\n", ReadTypeNames[rt], count, percentage)
		}
	}

	return nil
}

// debugCigar 调试函数，检查CIGAR中是否有X操作
func debugCigar(read *sam.Record) bool {
	hasX := false
	for _, cigarOp := range read.Cigar {
		/* 	if cigarOp.Type().String() == "X" {
			return true
		} */
		op := cigarOp.Type()
		// 在sam包中，CIGAR操作类型是整数，不是字符
		// 0=M, 1=I, 2=D, 3=N, 4=S, 5=H, 6=P, 7==, 8=X
		if op == 8 { // 8代表X操作
			hasX = true
			// 打印详细信息以便调试
			// fmt.Printf("  发现X操作: CIGAR=%v, 位置=%d, 操作类型=%d\n",
			// read.Cigar, read.Pos, op)
		}
	}
	return hasX
}

// findBAMFiles 查找所有BAM文件
func findBAMFiles() ([]string, error) {
	var bamFiles []string

	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		for _, sampleName := range sampleOrder {
			sortBam := filepath.Join(inputDir, "samples", sampleName, sampleName+".sorted.bam")
			filterBam := filepath.Join(inputDir, sampleName, sampleName+".filter.bam")
			if _, err := os.Stat(sortBam); err == nil {
				bamFiles = append(bamFiles, sortBam)
			} else if _, err := os.Stat(filterBam); err == nil {
				bamFiles = append(bamFiles, filterBam)

			} else {
				fmt.Printf("警告: 找不到样本 %s 的BAM文件: %s or %s\n", sampleName, sortBam, filterBam)
			}
		}
	} else {
		// 原来的方法：递归查找所有BAM文件
		err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				if strings.HasSuffix(path, ".sorted.bam") {
					bamFiles = append(bamFiles, path)
				} else if strings.HasSuffix(path, ".filter.bam") {
					bamFiles = append(bamFiles, path)
				}
			}

			return nil
		})

		if err != nil {
			return nil, err
		}

		// 按路径排序
		sort.Strings(bamFiles)
	}

	return bamFiles, nil
}

func main() {
	// 解析命令行参数
	flag.Parse()

	// 验证必需参数
	if inputDir == "" {
		fmt.Println("错误: 必需参数缺失")
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 检查输入目录是否存在
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		fmt.Printf("错误: 输入目录不存在: %s\n", inputDir)
		os.Exit(1)
	}

	// 如果指定了Excel文件，读取样本顺序
	if excelFile != "" {
		fmt.Printf("读取Excel文件: %s\n", excelFile)
		var err error
		sampleOrder, err = readExcelSampleOrder(excelFile)
		if err != nil {
			fmt.Printf("错误: 读取Excel文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("从Excel读取到 %d 个样本\n", len(sampleOrder))
	}

	// 创建输出目录
	if outputDir == "" {
		outputDir = filepath.Join(inputDir, "mutation_stats")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// 查找所有BAM文件
	bamFiles, err := findBAMFiles()
	if err != nil {
		fmt.Printf("查找BAM文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("找到 %d 个BAM文件\n", len(bamFiles))

	// 初始化统计
	stats := NewMutationStats()

	// 使用WaitGroup处理并发
	var wg sync.WaitGroup

	// 处理每个BAM文件
	for _, bamPath := range bamFiles {
		wg.Add(1)

		go func(path string) {
			defer wg.Done()

			// 提取样本名
			sampleDir := filepath.Dir(path)
			sampleName := filepath.Base(sampleDir)

			// 处理BAM文件
			if err := processBAMFile(path, sampleName, stats); err != nil {
				fmt.Printf("处理文件 %s 失败: %v\n", path, err)
			}
		}(bamPath)
	}

	// 等待所有goroutine完成
	wg.Wait()

	fmt.Println("\n开始生成统计文件...")

	// 写入各bam各位置各碱基变化组合的个数分布统计
	if err := writePositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入位置统计失败: %v\n", err)
	}

	// 写入各bam各碱基变化组合的分布
	if err := writeSampleMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入样本突变统计失败: %v\n", err)
	}

	// 写入总碱基变化组合的分布
	if err := writeTotalMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入总突变统计失败: %v\n", err)
	}

	// 写入read类型统计
	if err := writeReadTypeStats(stats, outputDir); err != nil {
		fmt.Printf("写入read类型统计失败: %v\n", err)
	}
	// 写入汇总报告
	if err := writeSummaryReport(stats, outputDir); err != nil {
		fmt.Printf("写入汇总报告失败: %v\n", err)
	}

	// 打印总体统计
	fmt.Printf("\n处理完成!\n")
	fmt.Printf("总体统计:\n")
	fmt.Printf("  总reads数: %d\n", stats.TotalReadCount)
	fmt.Printf("  总包含突变reads数: %d\n", stats.TotalReadsWithMuts)
	fmt.Printf("  Read类型分布:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		if count > 0 {
			percentage := float64(count) / float64(stats.TotalReadCount) * 100
			fmt.Printf("    %s: %d (%.2f%%)\n", ReadTypeNames[rt], count, percentage)
		}
	}
}
