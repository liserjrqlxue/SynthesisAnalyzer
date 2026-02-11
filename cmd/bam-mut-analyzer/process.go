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

// processBAMFile 处理单个BAM文件 - 使用新的数据结构
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

	// 获取或创建样本统计
	sampleStats := stats.getOrCreateSampleStats(sampleName)

	var totalReads, alignedReads, readsWithMutations, totalMutations int

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

		// 获取MD字符串
		mdTag, hasMD := read.Tag([]byte{'M', 'D'})
		mdStr := ""
		if hasMD {
			mdStr = mdTag.String()
			mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
		}

		// 分析详细的read类型
		readInfo := analyzeReadDetailedInfo(read, mdStr)

		// 更新样本统计
		sampleStats.Lock()

		// 更新read类型统计
		sampleStats.ReadTypeCounts[readInfo.MainType]++

		// 更新详细类型统计
		typeKey := getDetailedTypeKeyFromInfo(readInfo)
		sampleStats.DetailedTypeCounts[typeKey]++

		// 统计插入信息
		if readInfo.InsertSub != nil {
			for _, insertion := range readInfo.InsertSub.Insertions {
				// 插入长度分布
				length := insertion.Length
				sampleStats.InsertLengthDist[length]++

				// 插入序列统计
				if insertion.Bases != "" {
					sampleStats.InsertBaseCounts[insertion.Bases]++
				}

				// 碱基维度：插入碱基数
				sampleStats.MutationBaseCounts["insertion"] += insertion.Length
			}
		}

		// 统计缺失信息
		if readInfo.DeleteSub != nil {
			// 缺失长度分布
			for _, deletion := range readInfo.DeleteSub.Deletions {
				length := deletion.Length
				sampleStats.DeleteLengthDist[length]++

				// 碱基维度：缺失碱基数
				sampleStats.MutationBaseCounts["deletion"] += deletion.Length

				// 单碱基缺失统计
				if deletion.Length == 1 && len(deletion.Bases) == 1 {
					base := deletion.Bases[0]
					sampleStats.DeleteBaseCounts[base]++

					// 缺失位置统计
					posKey := fmt.Sprintf("%d:%c", deletion.Position, base)
					sampleStats.DeletePositionCounts[posKey]++
				}
			}
		}

		mutationCount := len(readInfo.Mutations)
		if mutationCount > 0 {
			readsWithMutations++
		}

		// 碱基维度：替换计数
		totalMutations += mutationCount
		sampleStats.MutationBaseCounts["substitution"] += mutationCount

		for _, mut := range readInfo.Mutations {
			// 创建键
			posKey := fmt.Sprintf("%d", mut.Position)
			mutKey := fmt.Sprintf("%s>%s", mut.Ref, mut.Alt)
			posMutKey := fmt.Sprintf("%s_%s", posKey, mutKey)

			// 更新位置细节
			if _, ok := sampleStats.PositionDetails[posKey]; !ok {
				sampleStats.PositionDetails[posKey] = make(map[string]int)
			}
			sampleStats.PositionDetails[posKey][mutKey]++

			// 更新样本突变统计
			sampleStats.Mutations[mutKey]++

			// 更新位置突变统计
			sampleStats.PositionMutations[posMutKey]++

		}
		// 保存突变到列表（可选）
		sampleStats.MutationList = append(sampleStats.MutationList, readInfo.Mutations...)

		sampleStats.Unlock()
	}

	// 更新样本统计
	sampleStats.Lock()
	sampleStats.ReadCounts = totalReads
	sampleStats.AlignedReads = alignedReads
	sampleStats.ReadsWithMutations = readsWithMutations
	// 合并突变统计
	for _, count := range sampleStats.Mutations {
		sampleStats.TotalMutations += count
	}
	sampleStats.Unlock()

	// 更新reads计数
	stats.Lock()
	stats.TotalReadCount += totalReads
	stats.TotalAlignedReads += alignedReads
	stats.TotalReadsWithMuts += readsWithMutations

	// 合并样本统计到总统计
	sampleStats.RLock()

	// 合并read类型统计
	for rt, count := range sampleStats.ReadTypeCounts {
		stats.TotalReadTypeCounts[rt] += count
	}

	// 合并详细类型统计
	for typeKey, count := range sampleStats.DetailedTypeCounts {
		stats.TotalDetailedTypeCounts[typeKey] += count
	}

	// 合并插入长度分布
	for length, count := range sampleStats.InsertLengthDist {
		stats.TotalInsertLengthDist[length] += count
	}

	// 合并缺失长度分布
	for length, count := range sampleStats.DeleteLengthDist {
		stats.TotalDeleteLengthDist[length] += count
	}

	// 合并缺失碱基统计
	for base, count := range sampleStats.DeleteBaseCounts {
		stats.TotalDeleteBaseCounts[base] += count
	}

	// 合并插入序列统计
	for seq, count := range sampleStats.InsertBaseCounts {
		stats.TotalInsertBaseCounts[seq] += count
	}

	// 合并缺失位置统计
	for posKey, count := range sampleStats.DeletePositionCounts {
		stats.TotalDeletePositionCounts[posKey] += count
	}

	// 合并突变统计
	for mutKey, count := range sampleStats.Mutations {
		stats.TotalMutations[mutKey] += count
	}

	sampleStats.RUnlock()
	stats.Unlock()

	// 计算比对率
	alignmentRate := 0.0
	if totalReads > 0 {
		alignmentRate = float64(alignedReads) / float64(totalReads) * 100
	}

	fmt.Printf("  总reads数: %d\n", totalReads)
	fmt.Printf("  比对reads数: %d (%.2f%%)\n", alignedReads, alignmentRate)
	fmt.Printf("  包含突变的reads数: %d\n", readsWithMutations)
	fmt.Printf("  总突变数: %d\n", totalMutations)
	fmt.Printf("  Read类型统计:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := sampleStats.ReadTypeCounts[rt]
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
func findBAMFiles(inputDir string) ([]string, error) {
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
