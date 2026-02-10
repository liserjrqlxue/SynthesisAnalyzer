package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

	stats := processBAMFiles(bamFiles)

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

	// 新增：写入详细统计
	if err := writeDetailedStats(stats, outputDir); err != nil {
		fmt.Printf("写入详细统计失败: %v\n", err)
	}

	// 新增：写入碱基维度统计
	if err := writeBaseMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入碱基维度统计失败: %v\n", err)
	}

	// 新增：写入缺失位置统计
	if err := writeDeletionPositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入缺失位置统计失败: %v\n", err)
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
