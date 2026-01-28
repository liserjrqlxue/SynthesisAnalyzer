package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// parseCigarX 专门解析CIGAR中的X操作
func parseCigarX(read *sam.Record) []Mutation {
	var mutations []Mutation

	// 检查是否有MD标签
	_, hasMD := read.Tag([]byte{'M', 'D'})
	if !hasMD {
		return mutations
	}

	// 获取参考起始位置（1-based）
	refStart := int(read.Pos)

	// 获取read序列
	seq := read.Seq.Expand()
	if len(seq) == 0 {
		return mutations
	}

	// 解析MD标签获取参考碱基信息
	mdTag, _ := read.Tag([]byte{'M', 'D'})
	mdStr := mdTag.String()

	// 去掉MD:Z:前缀
	mdStr = strings.TrimPrefix(mdStr, "MD:Z:")

	// 遍历CIGAR操作
	refPos := refStart - 1 // 转换为0-based，用于位置计算
	readPos := 0

	// 计数器，用于调试
	xCount := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type() // 这是整数，不是字符
		length := cigarOp.Len()

		switch op {
		case 0, 7, 8: // 0=M, 7==, 8=X
			// 处理匹配/错配
			for i := 0; i < length; i++ {
				currentPos := refPos + i + 1 // 1-based位置

				// 如果是X操作，记录突变
				if op == 8 {
					xCount++

					// 我们需要获取参考碱基和替代碱基
					refBase := getRefBaseFromMD(mdStr, currentPos-refStart+1)

					// 获取read碱基
					altBase := "N"
					if readPos+i < len(seq) {
						altBase = string(seq[readPos+i])
					}

					// 记录所有X操作，即使refBase或altBase是N
					// 因为我们知道X操作一定代表一个错配
					mutations = append(mutations, Mutation{
						Position: currentPos,
						Ref:      refBase,
						Alt:      altBase,
					})
				}
			}

			refPos += length
			readPos += length

		case 1: // 1=I (插入)
			// 插入，只增加read位置
			readPos += length

		case 2, 3: // 2=D, 3=N (删除或跳过)
			// 删除，只增加参考位置
			refPos += length

		case 4: // 4=S (软剪接)
			// 软剪接，只增加read位置
			readPos += length

		case 5, 6: // 5=H, 6=P (硬剪接或填充)
			// 硬剪接或填充，不处理
		}
	}

	return mutations
}

// getRefBaseFromMD 从MD字符串获取指定位置的参考碱基
func getRefBaseFromMD(mdStr string, pos int) string {
	if mdStr == "" {
		return "N"
	}

	// 当前位置（相对于refStart，1-based）
	currentPos := 0
	i := 0

	for i < len(mdStr) {
		// 读取数字（匹配长度）
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				i = j
				continue
			}

			// 检查位置是否在这个匹配区域内
			if pos > currentPos && pos <= currentPos+num {
				// 在匹配区域内，参考碱基是未知的
				// 但我们可以从上下文中推断，或者返回占位符
				return "N"
			}

			currentPos += num
			i = j
		} else if isBase(mdStr[i]) {
			// 错配（碱基）
			currentPos++
			if currentPos == pos {
				// 找到对应位置的错配
				return string(mdStr[i])
			}
			i++
		} else if mdStr[i] == '^' {
			// 删除
			i++
			// 跳过删除的碱基
			for i < len(mdStr) && isBase(mdStr[i]) {
				// 删除的位置在参考序列上，但不在read中
				currentPos++
				if currentPos == pos {
					// 如果位置在删除区域，参考碱基是删除的碱基
					return string(mdStr[i])
				}
				i++
			}
		} else {
			i++
		}
	}

	return "N"
}

// parseMDString 解析MD字符串，返回位置到参考碱基的映射
func parseMDString(mdStr string, refStart int) map[int]string {
	refMap := make(map[int]string)

	if mdStr == "" {
		return refMap
	}

	// 当前位置（相对于refStart，1-based）
	posOffset := 0
	i := 0

	for i < len(mdStr) {
		// 读取数字（匹配长度）
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err == nil {
				posOffset += num
			}
			i = j
		} else if isBase(mdStr[i]) {
			// 错配（碱基）
			position := refStart + posOffset
			refMap[position] = string(mdStr[i])
			posOffset++
			i++
		} else if mdStr[i] == '^' {
			// 删除
			i++
			// 跳过删除的碱基
			for i < len(mdStr) && isBase(mdStr[i]) {
				posOffset++
				i++
			}
		} else {
			i++
		}
	}

	return refMap
}

// inferRefBaseFromContext 从MD字符串上下文推断参考碱基
func inferRefBaseFromContext(mdStr string, pos int) string {
	// 简化实现：返回N表示未知
	return "N"
}

// isBase 判断字符是否为有效碱基
func isBase(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// parseCigarAndMD 解析CIGAR和MD标签 - 只关注CIGAR中的X操作
func parseCigarAndMD(read *sam.Record) []Mutation {
	var mutations []Mutation

	// 获取MD标签
	mdTag, found := read.Tag([]byte{'M', 'D'})
	if !found {
		return mutations
	}

	// 提取MD字符串
	mdStr := mdTag.String()
	const mdPrefix = "MD:Z:"
	if strings.HasPrefix(mdStr, mdPrefix) {
		mdStr = mdStr[len(mdPrefix):]
	}

	if mdStr == "" {
		return mutations
	}

	// 解析MD字符串构建参考序列信息
	refStart := int(read.Pos) - 1
	seq := read.Seq.Expand()

	// 解析MD字符串，构建一个映射：参考位置 -> 参考碱基（只记录错配和匹配）
	refMap := make(map[int]byte)

	i := 0
	refPos := refStart

	for i < len(mdStr) {
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			// 匹配长度
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				i = j
				continue
			}

			// 匹配区域，不记录到refMap
			refPos += num
			i = j
		} else if (mdStr[i] >= 'A' && mdStr[i] <= 'Z') || (mdStr[i] >= 'a' && mdStr[i] <= 'z') {
			// 错配，记录参考碱基
			refMap[refPos] = mdStr[i]
			refPos++
			i++
		} else if mdStr[i] == '^' {
			// 删除
			i++
			for i < len(mdStr) && ((mdStr[i] >= 'A' && mdStr[i] <= 'Z') || (mdStr[i] >= 'a' && mdStr[i] <= 'z')) {
				// 删除的碱基不记录到refMap
				refPos++
				i++
			}
		} else {
			i++
		}
	}

	// 遍历CIGAR操作，只处理X操作
	refOffset := 0
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		switch op {
		case 'M', '=':
			refOffset += length
			readPos += length

		case 'X':
			// CIGAR中的X操作
			for j := 0; j < length; j++ {
				position := refStart + refOffset + 1 // 1-based

				// 获取参考碱基
				var refBase byte
				if base, ok := refMap[refStart+refOffset]; ok {
					refBase = base
				} else {
					// 如果不在refMap中，尝试从序列推断
					if readPos < len(seq) {
						refBase = seq[readPos]
					} else {
						refBase = 'N'
					}
				}

				// 获取替代碱基
				var altBase byte
				if readPos < len(seq) {
					altBase = seq[readPos]
				} else {
					altBase = 'N'
				}

				if refBase != altBase {
					mutations = append(mutations, Mutation{
						Position: position,
						Ref:      string(refBase),
						Alt:      string(altBase),
					})
				}

				refOffset++
				readPos++
			}

		case 'I':
			readPos += length

		case 'D', 'N':
			refOffset += length

		case 'S':
			readPos += length

		case 'H', 'P':
			// 忽略
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

	var totalReads, readsWithMutations int

	var totalXReads int
	var debugCount int
	var totalXOps int
	var mutationsFound int

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

		// 调试：检查是否有X操作
		hasX := debugCigar(read)
		if hasX {
			totalXReads++

			/* 		// 只打印前几个有X操作的read进行调试
			if debugCount < 5 {
				fmt.Printf("调试: 发现CIGAR X操作, CIGAR: %v, POS: %d\n",
					read.Cigar, read.Pos)
				debugCount++
			} */
		}

		// 统计X操作
		xCount := countXOperations(read)
		if xCount > 0 {
			readsWithMutations++
			totalXOps += xCount

			// 解析突变
			mutations := parseCigarXWithMD(read)
			mutationsFound += len(mutations)

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

	// 打印调试信息

	fmt.Printf("  总reads数: %d\n", totalReads)
	fmt.Printf("  [DEBUG]有CIGAR X操作的reads数: %d\n", totalXReads)
	fmt.Printf("  包含突变的reads数: %d\n", readsWithMutations)
	fmt.Printf("  总X操作数: %d\n", totalXOps)
	fmt.Printf("  解析到的突变数: %d\n", mutationsFound)

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
