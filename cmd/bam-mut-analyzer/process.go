package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
)

// processBAMFile 处理单个BAM文件 - 更新版本，调用updateBaseMutationCounts
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
	var alignedReads int

	var totalXReads int
	var debugCount int
	var totalXOps int

	// 初始化样本的read type统计
	stats.Lock()
	if _, ok := stats.SampleReadTypeCounts[sampleName]; !ok {
		stats.SampleReadTypeCounts[sampleName] = make(map[ReadType]int)
	}
	if _, ok := stats.SampleDetailedTypeCounts[sampleName]; !ok {
		stats.SampleDetailedTypeCounts[sampleName] = make(map[string]int)
	}
	if _, ok := stats.SampleInsertLengthDist[sampleName]; !ok {
		stats.SampleInsertLengthDist[sampleName] = make(map[int]int)
	}
	if _, ok := stats.SampleDeleteLengthDist[sampleName]; !ok {
		stats.SampleDeleteLengthDist[sampleName] = make(map[int]int)
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
		alignedReads++

		// 分析read类型
		readType := analyzeReadType(read)

		// 更新read type统计
		stats.Lock()
		stats.SampleReadTypeCounts[sampleName][readType]++
		stats.TotalReadTypeCounts[readType]++
		stats.Unlock()

		// 获取MD字符串
		mdTag, hasMD := read.Tag([]byte{'M', 'D'})
		mdStr := ""
		if hasMD {
			mdStr = mdTag.String()
			mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
		}

		// 分析详细的read类型
		detailedType := analyzeDetailedReadTypeWithMD(read, mdStr)

		// 更新详细类型统计
		typeKey := getDetailedTypeKey(detailedType)
		stats.Lock()
		stats.SampleDetailedTypeCounts[sampleName][typeKey]++
		stats.TotalDetailedTypeCounts[typeKey]++
		stats.Unlock()

		// 更新主类型统计
		stats.Lock()
		stats.SampleReadTypeCounts[sampleName][detailedType.MainType]++
		stats.TotalReadTypeCounts[detailedType.MainType]++
		stats.Unlock()

		// 统计插入长度分布
		if detailedType.InsertSub != nil && len(detailedType.InsertSub.Insertions) > 0 {
			for _, insertion := range detailedType.InsertSub.Insertions {
				length := insertion.Length
				stats.Lock()
				stats.SampleInsertLengthDist[sampleName][length]++
				stats.TotalInsertLengthDist[length]++
				stats.Unlock()
			}
		}

		// 统计缺失长度分布
		if detailedType.DeleteSub != nil && len(detailedType.DeleteSub.Deletions) > 0 {
			// 缺失长度分布
			for _, deletion := range detailedType.DeleteSub.Deletions {
				length := deletion.Length
				stats.Lock()
				stats.SampleDeleteLengthDist[sampleName][length]++
				stats.TotalDeleteLengthDist[length]++
				stats.Unlock()
			}
		}

		// 解析突变
		mutations := parseMutationsWithMD(read)

		// 更新碱基维度的变异计数
		updateBaseMutationCounts(read, mutations, detailedType.InsertSub, detailedType.DeleteSub, stats, sampleName)

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

			// 更新突变统计
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
	stats.SampleAlignedReads[sampleName] = alignedReads
	stats.SampleReadsWithMuts[sampleName] = readsWithMutations
	stats.TotalReadCount += totalReads
	stats.TotalAlignedReads += alignedReads
	stats.TotalReadsWithMuts += readsWithMutations
	stats.Unlock()

	// 打印read type统计
	stats.RLock()
	readTypeCounts := stats.SampleReadTypeCounts[sampleName]
	stats.RUnlock()

	// 输出比对率
	alignmentRate := 0.0
	if totalReads > 0 {
		alignmentRate = float64(alignedReads) / float64(totalReads) * 100
	}

	fmt.Printf("  总reads数: %d\n", totalReads)
	fmt.Printf("  比对reads数: %d (%.2f%%)\n", alignedReads, alignmentRate)
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

func processBAMFiles(bamFiles []string) (stats *MutationStats) {
	// 初始化统计
	stats = NewMutationStats()

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

	return stats
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
