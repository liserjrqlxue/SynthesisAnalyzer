package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
)

// MutationStats 存储突变统计
type MutationStats struct {
	sync.RWMutex
	PositionMutations map[string]map[string]int            // sample -> position:mutation -> count
	SampleMutations   map[string]map[string]int            // sample -> mutation -> count
	TotalMutations    map[string]int                       // mutation -> total count
	PositionDetails   map[string]map[string]map[string]int // sample -> position -> mutation -> count
}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	return &MutationStats{
		PositionMutations: make(map[string]map[string]int),
		SampleMutations:   make(map[string]map[string]int),
		TotalMutations:    make(map[string]int),
		PositionDetails:   make(map[string]map[string]map[string]int),
	}
}

// parseCigarAndMD 解析CIGAR和MD标签
func parseCigarAndMD(read *sam.Record) []Mutation {
	var mutations []Mutation

	// 获取MD标签
	mdTag, found := read.Tag([]byte{'M', 'D'})
	if !found {
		return mutations
	}

	mdStr, ok := mdTag.(string)
	if !ok {
		return mutations
	}

	// 解析CIGAR
	refPos := int(read.Pos) - 1 // 0-based
	readPos := 0
	seq := read.Seq.Expand()

	// 解析MD字符串
	var mdOps []MDOp
	i := 0
	for i < len(mdStr) {
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
				j++
			}
			num := parseNumber(mdStr[i:j])
			mdOps = append(mdOps, MDOp{Type: 'M', Length: num})
			i = j
		} else if mdStr[i] == '^' {
			j := i + 1
			for j < len(mdStr) && !isDigit(mdStr[j]) {
				j++
			}
			bases := mdStr[i+1 : j]
			mdOps = append(mdOps, MDOp{Type: 'D', Bases: bases})
			i = j
		} else {
			mdOps = append(mdOps, MDOp{Type: 'X', Base: mdStr[i]})
			i++
		}
	}

	// 遍历CIGAR操作
	mdIdx := 0
	mdOffset := 0

	for _, cigarOp := range read.Cigar {
		switch cigarOp.Type() {
		case 'M', '=', 'X':
			for k := 0; k < cigarOp.Len(); k++ {
				// 处理当前位置的MD操作
				for mdIdx < len(mdOps) {
					mdOp := mdOps[mdIdx]

					if mdOp.Type == 'M' {
						if mdOffset < mdOp.Length {
							// 匹配位置
							mdOffset++
							break
						} else {
							mdIdx++
							mdOffset = 0
							continue
						}
					} else if mdOp.Type == 'X' {
						// 错配位置
						if cigarOp.Type() == 'X' {
							// CIGAR中的X操作
							refBase := mdOp.Base
							if readPos < len(seq) {
								altBase := seq[readPos]
								mutations = append(mutations, Mutation{
									Position: refPos + 1, // 1-based
									Ref:      string(refBase),
									Alt:      string(altBase),
								})
							}
						}
						mdIdx++
						break
					} else if mdOp.Type == 'D' {
						if mdOffset < len(mdOp.Bases) {
							mdOffset++
						} else {
							mdIdx++
							mdOffset = 0
							continue
						}
					}
				}

				if cigarOp.Type() != 'D' {
					readPos++
				}
				refPos++
			}

		case 'I':
			readPos += cigarOp.Len()

		case 'D', 'N':
			refPos += cigarOp.Len()

		case 'S':
			readPos += cigarOp.Len()

		case 'H', 'P':
			// 硬剪接和填充，不处理
		}
	}

	return mutations
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

		// 解析突变
		mutations := parseCigarAndMD(read)

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

	fmt.Printf("  总reads数: %d\n", totalReads)
	fmt.Printf("  包含突变的reads数: %d\n", readsWithMutations)
	fmt.Printf("  总突变数: %d\n", totalMutations)

	return nil
}

// findBAMFiles 查找所有BAM文件
func findBAMFiles(inputDir string) ([]string, error) {
	var bamFiles []string

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".sorted.bam") {
			bamFiles = append(bamFiles, path)
		}

		return nil
	})

	return bamFiles, err
}

// writePositionStats 写入各bam各位置各碱基变化组合的个数分布统计
func writePositionStats(stats *MutationStats, outputDir string) error {
	for sampleName, posDetails := range stats.PositionDetails {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_position_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Position,Ref>Alt,Count\n")

		// 收集所有位置
		var positions []string
		for pos := range posDetails {
			positions = append(positions, pos)
		}

		// 按位置排序
		sort.Slice(positions, func(i, j int) bool {
			return parseNumber(positions[i]) < parseNumber(positions[j])
		})

		for _, pos := range positions {
			// 收集该位置的所有突变
			var mutations []string
			for mut := range posDetails[pos] {
				mutations = append(mutations, mut)
			}
			sort.Strings(mutations)

			for _, mut := range mutations {
				count := posDetails[pos][mut]
				writer.WriteString(fmt.Sprintf("%s,%s,%d\n", pos, mut, count))
			}
		}

		writer.Flush()
		fmt.Printf("  已写入: %s\n", filename)
	}

	return nil
}

// writeSampleMutationStats 写入各bam各碱基变化组合的分布
func writeSampleMutationStats(stats *MutationStats, outputDir string) error {
	for sampleName, mutCounts := range stats.SampleMutations {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_mutation_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Ref>Alt,Count\n")

		// 收集所有突变
		var mutations []string
		for mut := range mutCounts {
			mutations = append(mutations, mut)
		}
		sort.Strings(mutations)

		for _, mut := range mutations {
			count := mutCounts[mut]
			writer.WriteString(fmt.Sprintf("%s,%d\n", mut, count))
		}

		writer.Flush()
		fmt.Printf("  已写入: %s\n", filename)
	}

	return nil
}

// writeTotalMutationStats 写入总碱基变化组合的分布
func writeTotalMutationStats(stats *MutationStats, outputDir string) error {
	filename := filepath.Join(outputDir, "total_mutation_stats.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Ref>Alt,Count\n")

	// 收集所有突变
	var mutations []string
	for mut := range stats.TotalMutations {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	for _, mut := range mutations {
		count := stats.TotalMutations[mut]
		writer.WriteString(fmt.Sprintf("%s,%d\n", mut, count))
	}

	writer.Flush()
	fmt.Printf("总统计已写入: %s\n", filename)

	return nil
}

// writeSummaryReport 写入汇总报告
func writeSummaryReport(stats *MutationStats, outputDir string) error {
	// 1. 样本汇总
	filename := filepath.Join(outputDir, "summary_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 收集所有突变类型
	allMutations := make(map[string]bool)
	for _, sampleMuts := range stats.SampleMutations {
		for mut := range sampleMuts {
			allMutations[mut] = true
		}
	}

	// 转换为排序的切片
	var mutationTypes []string
	for mut := range allMutations {
		mutationTypes = append(mutationTypes, mut)
	}
	sort.Strings(mutationTypes)

	// 写入表头
	header := "Sample,Total_Mutations"
	for _, mut := range mutationTypes {
		header += "," + mut
	}
	writer.WriteString(header + "\n")

	// 收集样本名并排序
	var sampleNames []string
	for sampleName := range stats.SampleMutations {
		sampleNames = append(sampleNames, sampleName)
	}
	sort.Strings(sampleNames)

	// 写入每行数据
	for _, sampleName := range sampleNames {
		sampleMuts := stats.SampleMutations[sampleName]
		total := 0
		for _, count := range sampleMuts {
			total += count
		}

		row := fmt.Sprintf("%s,%d", sampleName, total)
		for _, mut := range mutationTypes {
			count := sampleMuts[mut]
			row += fmt.Sprintf(",%d", count)
		}
		writer.WriteString(row + "\n")
	}

	writer.Flush()
	fmt.Printf("样本汇总已写入: %s\n", filename)

	// 2. 突变频谱
	filename = filepath.Join(outputDir, "mutation_spectrum.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Mutation,Count,Frequency\n")

	var mutations []string
	for mut := range stats.TotalMutations {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	total := 0
	for _, count := range stats.TotalMutations {
		total += count
	}

	for _, mut := range mutations {
		count := stats.TotalMutations[mut]
		frequency := float64(count) / float64(total)
		writer.WriteString(fmt.Sprintf("%s,%d,%.6f\n", mut, count, frequency))
	}

	writer.Flush()
	fmt.Printf("突变频谱已写入: %s\n", filename)

	return nil
}

func main() {
	// 解析命令行参数
	if len(os.Args) < 3 {
		fmt.Println("使用方法: bam-mut-analyzer <输入目录> <输出目录>")
		fmt.Println("示例: bam-mut-analyzer input-0116B-2.splitter/samples mutation_stats")
		os.Exit(1)
	}

	inputDir := os.Args[1]
	outputDir := os.Args[2]

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		return
	}

	// 查找所有BAM文件
	bamFiles, err := findBAMFiles(inputDir)
	if err != nil {
		fmt.Printf("查找BAM文件失败: %v\n", err)
		return
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

	// 写入汇总报告
	if err := writeSummaryReport(stats, outputDir); err != nil {
		fmt.Printf("写入汇总报告失败: %v\n", err)
	}

	fmt.Println("\n处理完成!")
}

// 简化版本，使用samtools命令行工具
func simpleVersion() {
	// 这是一个简化的shell脚本版本，可以快速处理
	script := `#!/bin/bash
INPUT_DIR="input-0116B-2.splitter/samples"
OUTPUT_DIR="mutation_stats_simple"
mkdir -p $OUTPUT_DIR

echo "开始处理BAM文件..."

for bam in $INPUT_DIR/*/*.sorted.bam; do
    SAMPLE=$(basename $(dirname $bam))
    echo "处理样本: $SAMPLE"
    
    # 提取MD标签并统计
    samtools view $bam | grep -o "MD:Z:[^[:space:]]*" | cut -d: -f3 | \
    awk '{
        md = $1
        pos = 1
        while(match(md, /[0-9]+[ACGTN]/)) {
            start = RSTART
            len = RLENGTH
            num = substr(md, start, len-1)
            base = substr(md, start+len-1, 1)
            pos += num
            print pos ":" base
            md = substr(md, start+len)
            pos++
        }
    }' > $OUTPUT_DIR/${SAMPLE}_mutations.txt
    
    # 统计突变
    cat $OUTPUT_DIR/${SAMPLE}_mutations.txt | sort | uniq -c | \
    awk '{print $2","$1}' > $OUTPUT_DIR/${SAMPLE}_mutation_stats.csv
done

echo "生成汇总统计..."
cat $OUTPUT_DIR/*_mutation_stats.csv | awk -F, '{
    counts[$1]+=$2
} END {
    for (mut in counts) {
        print mut "," counts[mut]
    }
}' | sort > $OUTPUT_DIR/total_mutation_stats.csv

echo "处理完成! 结果保存在 $OUTPUT_DIR/"
`

	fmt.Println("简化脚本:")
	fmt.Println(script)
}
