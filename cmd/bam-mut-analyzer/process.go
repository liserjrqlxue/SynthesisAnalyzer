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

// processBAMFile 处理单个BAM文件 - 添加细分类统计
func processBAMFile(bamPath, sampleName string, stats *MutationStats, refLenFromExcel, headCutFromExcel, tailCutFromExcel int) error {
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

	// 获取参考序列长度（优先从BAM header获取）
	refSeqLen := 0
	for _, ref := range br.Header().Refs() {
		if ref.Name() == sampleName {
			refSeqLen = ref.Len()
			break
		}
	}
	// 如果BAM中没有，使用Excel提供的长度
	if refSeqLen == 0 && refLenFromExcel > 0 {
		refSeqLen = refLenFromExcel
	}
	if refSeqLen == 0 {
		fmt.Printf("  警告: 无法获取样本 %s 的参考序列长度，头尾切除将不会应用\n", sampleName)
	} else if refSeqLen != refLenFromExcel {
		fmt.Printf("  警告: 样本 %s 的参考序列长度冲突：[%d vs. %d]\n", sampleName, refSeqLen, refLenFromExcel)
	}

	// 设置头尾切除长度：优先使用Excel提供的，否则用全局默认
	sampleHeadCut := headCut
	sampleTailCut := tailCut
	if headCutFromExcel > 0 {
		sampleHeadCut = headCutFromExcel
	}
	if tailCutFromExcel > 0 {
		sampleTailCut = tailCutFromExcel
	}

	var totalReads, alignedReads int
	var readsWithMutations, totalMutations int

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
		// mdStr, _ := getMDString(read)

		// ---------- 新增：位置详细统计 ----------
		refStart := int(read.Pos) // 0-based
		// 解析MD字符串，建立位置到参考碱基的映射
		mdMap := parseMDToMap(mdStr, refStart)

		refPos := refStart
		readPos := 0
		readIsPerfect := true
		perfectUptoNow := true
		cigarOps := read.Cigar

		for ci, cigarOp := range cigarOps {
			op := cigarOp.Type()
			length := cigarOp.Len()

			switch op {
			case 0, 7, 8: // M, =, X
				for j := 0; j < length; j++ {
					pos := refPos + j + 1 // 1-based
					isMismatch := false
					if op == 8 {
						isMismatch = true
					} else if hasMD {
						_, ok := mdMap[refPos+j]
						if ok {
							isMismatch = true
						}
					}
					// 插入只影响 M 操作的最后一个位置
					hasInsertAfter := false
					if j == length-1 && ci+1 < len(cigarOps) && cigarOps[ci+1].Type() == 1 {
						hasInsertAfter = true
					}

					sampleStats.Lock()
					detail, exists := sampleStats.PositionStats[pos]
					if !exists {
						detail = &PositionDetail{}
						sampleStats.PositionStats[pos] = detail
					}
					detail.Depth++

					if perfectUptoNow && !isMismatch && !hasInsertAfter {
						detail.PerfectUptoPosCount++
					}

					if hasInsertAfter {
						if isMismatch {
							detail.MismatchWithIns++
						} else {
							detail.MatchWithIns++
						}
						detail.Insertion++ // 该位置有一次插入事件
					} else {
						if isMismatch {
							detail.MismatchPure++
						} else {
							detail.MatchPure++
						}
					}
					sampleStats.Unlock()

					// 更新 read 完美状态：遇到错配、插入、缺失都不完美
					if isMismatch || hasInsertAfter {
						readIsPerfect = false
						perfectUptoNow = false
					}
				}
				refPos += length
				readPos += length

			case 2, 3: // D, N
				for j := 0; j < length; j++ {
					pos := refPos + j + 1
					sampleStats.Lock()
					detail, exists := sampleStats.PositionStats[pos]
					if !exists {
						detail = &PositionDetail{}
						sampleStats.PositionStats[pos] = detail
					}
					detail.Depth++
					detail.Deletion++
					sampleStats.Unlock()
				}
				readIsPerfect = false
				perfectUptoNow = false
				refPos += length

			case 1: // I
				readPos += length
				readIsPerfect = false
				perfectUptoNow = false
				// 插入本身不占用参考位置，但影响前一个位置的统计（已在 M 操作中处理）
			case 4: // S
				readPos += length
			}
		}

		// 整条read完美：为每个覆盖位置累加PerfectReadsCount
		if readIsPerfect {
			refPos = refStart
			readPos = 0
			for _, cigarOp := range cigarOps {
				op := cigarOp.Type()
				length := cigarOp.Len()
				switch op {
				case 0, 7, 8:
					for j := 0; j < length; j++ {
						pos := refPos + j + 1
						sampleStats.Lock()
						detail, exists := sampleStats.PositionStats[pos]
						if !exists {
							detail = &PositionDetail{}
							sampleStats.PositionStats[pos] = detail
						}
						detail.PerfectReadsCount++
						sampleStats.Unlock()
					}
					refPos += length
					readPos += length
				case 2, 3:
					refPos += length
				case 1, 4:
					readPos += length
				}
			}
		}

		// 分析详细的read类型
		readInfo := analyzeReadDetailedInfo(read, mdStr)

		// 统计突变信息
		mutationCount := len(readInfo.Mutations)
		sampleStats.Lock()
		sampleStats.SubstitutionCountDist[mutationCount]++
		sampleStats.Unlock()
		// 跳过非良好比对
		if mutationCount > maxSubstitutions {
			continue
		}
		sampleStats.GoodAlignedReads++

		// 更新样本统计
		sampleStats.Lock()

		// 更新read类型统计
		sampleStats.ReadTypeCounts[readInfo.MainType]++

		// 更新详细类型统计
		typeKey := getDetailedTypeKeyFromInfo(readInfo)
		sampleStats.DetailedTypeCounts[typeKey]++

		// 统计插入信息
		if readInfo.InsertSub != nil && len(readInfo.InsertSub.Insertions) > 0 {
			// reads维度：包含插入的reads数
			sampleStats.InsertReads++

			for _, insertion := range readInfo.InsertSub.Insertions {
				// 插入事件个数维度：每个插入事件计数
				sampleStats.InsertEventCount++

				// 插入碱基个数维度：累加插入碱基数
				sampleStats.InsertBaseTotal += insertion.Length
				// 碱基维度：插入碱基数
				sampleStats.MutationBaseCounts["insertion"] += insertion.Length

				// 插入长度分布
				length := insertion.Length
				if length > 3 {
					length = 4 // >3
				}
				sampleStats.InsertLengthDist[length]++

				// 插入序列统计
				if insertion.Bases != "" {
					sampleStats.InsertBaseCounts[insertion.Bases]++
				}

			}
		}

		// 统计缺失细分类（原代码中已有统计，现在需要添加 Del1 碱基分布）
		if readInfo.DeleteSub != nil && len(readInfo.DeleteSub.Deletions) > 0 {
			// reads维度：包含缺失的reads数
			sampleStats.DeleteReads++

			for _, deletion := range readInfo.DeleteSub.Deletions {
				// 缺失事件个数维度：每个缺失事件计数
				sampleStats.DeleteEventCount++

				// 缺失碱基个数维度：累加缺失碱基数
				sampleStats.DeleteBaseTotal += deletion.Length
				// 碱基维度：缺失碱基数
				sampleStats.MutationBaseCounts["deletion"] += deletion.Length

				// 缺失长度分布
				length := deletion.Length
				if length > 3 {
					length = 4 // >3
				}
				sampleStats.DeleteLengthDist[length]++

				// 单碱基缺失统计
				if deletion.Length == 1 && len(deletion.Bases) == 1 {
					base := deletion.Bases[0]
					sampleStats.Del1BaseCounts[base]++

					// 缺失位置统计
					posKey := fmt.Sprintf("%d:%c", deletion.Position, base)
					sampleStats.DeletePositionCounts[posKey]++
				}
			}
		}

		// 统计突变信息
		if mutationCount > 0 {
			readsWithMutations++
			totalMutations += mutationCount

			// reads维度：包含替换的reads数
			sampleStats.SubstitutionReads++

			// 替换事件个数维度：累加替换事件数
			sampleStats.SubstitutionEventCount += mutationCount

			// 替换碱基个数维度：累加替换碱基数（每个替换事件是一个碱基替换）
			sampleStats.SubstitutionBaseTotal += mutationCount
			// 碱基维度：替换计数
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
		}

		// 保存突变到列表（可选）
		sampleStats.MutationList = append(sampleStats.MutationList, readInfo.Mutations...)

		// ----- 新增：细分类统计 -----
		// 记录该read出现的细分类（用于组合统计）
		insertSubtypes := make(map[InsertionSubtype]bool)
		deleteSubtypes := make(map[DeletionSubtype]bool)
		substSubtypes := make(map[SubstitutionSubtype]bool)

		// 插入细分类
		if readInfo.InsertSub != nil {
			for _, ins := range readInfo.InsertSub.Insertions {
				st := ins.Subtype
				// 事件数
				sampleStats.InsertSubtypeEvents[st]++
				// 碱基数
				sampleStats.InsertSubtypeBases[st] += ins.Length
				insertSubtypes[st] = true
			}
			// reads数（每个细分类单独计）
			for st := range insertSubtypes {
				sampleStats.InsertSubtypeReads[st]++
			}
		}

		// 缺失细分类
		if readInfo.DeleteSub != nil {
			for _, del := range readInfo.DeleteSub.Deletions {
				st := del.Subtype
				sampleStats.DeleteSubtypeEvents[st]++
				sampleStats.DeleteSubtypeBases[st] += del.Length
				deleteSubtypes[st] = true
			}
			for st := range deleteSubtypes {
				sampleStats.DeleteSubtypeReads[st]++
			}
		}

		// 替换细分类
		if len(readInfo.Substitutions) > 0 {
			for _, sub := range readInfo.Substitutions {
				st := sub.Subtype
				sampleStats.SubstitutionSubtypeEvents[st]++
				sampleStats.SubstitutionSubtypeBases[st]++ // 替换碱基数为1
				substSubtypes[st] = true
			}
			for st := range substSubtypes {
				sampleStats.SubstitutionSubtypeReads[st]++
			}
		}

		// 细分类组合统计
		if len(insertSubtypes) > 0 || len(deleteSubtypes) > 0 || len(substSubtypes) > 0 {
			combKey := buildSubtypeCombinationKey(insertSubtypes, deleteSubtypes, substSubtypes)
			sampleStats.SubtypeCombinationCounts[combKey]++
		}

		sampleStats.Unlock()
	}

	// 更新样本统计
	sampleStats.Lock()
	sampleStats.ReadCounts = totalReads
	sampleStats.AlignedReads = alignedReads
	sampleStats.ReadsWithMutations = readsWithMutations
	sampleStats.RefLength = refSeqLen
	sampleStats.HeadCut = sampleHeadCut
	sampleStats.TailCut = sampleTailCut
	// 合并突变统计
	for _, count := range sampleStats.Mutations {
		sampleStats.TotalMutations += count
	}
	sampleStats.Unlock()

	// 更新总统计
	stats.Lock()
	stats.TotalReadCount += totalReads
	stats.TotalAlignedReads += alignedReads
	stats.TotalReadsWithMuts += readsWithMutations

	// 合并样本统计到总统计
	sampleStats.RLock()

	// 新增：合并reads维度变异统计
	stats.TotalInsertReads += sampleStats.InsertReads
	stats.TotalDeleteReads += sampleStats.DeleteReads
	stats.TotalSubstitutionReads += sampleStats.SubstitutionReads

	// 新增：合并变异事件个数维度
	stats.TotalInsertEventCount += sampleStats.InsertEventCount
	stats.TotalDeleteEventCount += sampleStats.DeleteEventCount
	stats.TotalSubstitutionEventCount += sampleStats.SubstitutionEventCount

	// 新增：合并碱基个数维度
	stats.TotalInsertBaseTotal += sampleStats.InsertBaseTotal
	stats.TotalDeleteBaseTotal += sampleStats.DeleteBaseTotal
	stats.TotalSubstitutionBaseTotal += sampleStats.SubstitutionBaseTotal

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
	for base, count := range sampleStats.Del1BaseCounts {
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

	for st, cnt := range sampleStats.DeleteSubtypeReads {
		stats.TotalDeleteSubtypeReads[st] += cnt
	}
	for st, cnt := range sampleStats.DeleteSubtypeEvents {
		stats.TotalDeleteSubtypeEvents[st] += cnt
	}
	for st, cnt := range sampleStats.DeleteSubtypeBases {
		stats.TotalDeleteSubtypeBases[st] += cnt
	}

	for st, cnt := range sampleStats.InsertSubtypeReads {
		stats.TotalInsertSubtypeReads[st] += cnt
	}
	for st, cnt := range sampleStats.InsertSubtypeEvents {
		stats.TotalInsertSubtypeEvents[st] += cnt
	}
	for st, cnt := range sampleStats.InsertSubtypeBases {
		stats.TotalInsertSubtypeBases[st] += cnt
	}

	for st, cnt := range sampleStats.SubstitutionSubtypeReads {
		stats.TotalSubstitutionSubtypeReads[st] += cnt
	}
	for st, cnt := range sampleStats.SubstitutionSubtypeEvents {
		stats.TotalSubstitutionSubtypeEvents[st] += cnt
	}
	for st, cnt := range sampleStats.SubstitutionSubtypeBases {
		stats.TotalSubstitutionSubtypeBases[st] += cnt
	}

	// ... 类似合并插入、替换细分类 ...
	for comb, cnt := range sampleStats.SubtypeCombinationCounts {
		stats.TotalSubtypeCombinationCounts[comb] += cnt
	}

	for count, n := range sampleStats.SubstitutionCountDist {
		stats.TotalSubstitutionCountDist[count] += n
	}

	// 合并 Del1 碱基分布
	for base, cnt := range sampleStats.Del1BaseCounts {
		stats.TotalDel1BaseCounts[base] += cnt
	}

	stats.TotalGoodAlignedReads += sampleStats.GoodAlignedReads
	// 累加 RefLength * GoodAlignedReads
	stats.TotalRefLengthGoodAligned += (sampleStats.RefLength - sampleStats.HeadCut - sampleStats.TailCut) * sampleStats.GoodAlignedReads

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

			refLen := 0
			headCutFromExcel := 0
			tailCutFromExcel := 0
			if fullLengths != nil {
				refLen = fullLengths[sampleName]
				headCutFromExcel = headCuts[sampleName]
				tailCutFromExcel = tailCuts[sampleName]
			}

			// 处理BAM文件
			if err := processBAMFile(path, sampleName, stats,
				refLen, headCutFromExcel, tailCutFromExcel); err != nil {
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
