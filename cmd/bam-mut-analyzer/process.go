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

// processBAMFile 处理单个BAM文件 - 添加细分类统计
func processBAMFile(bamPath, sampleName string, stats *MutationStats, refLenFromExcel, headCutFromExcel, tailCutFromExcel int, fullSeqFromExcel string) error {
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

	// 计算切除头尾后的参考序列ACGT计数
	if fullSeqFromExcel != "" {
		sampleStats.Lock()

		sampleStats.RefSeqFull = fullSeqFromExcel
		// 确定切除长度（优先Excel，否则全局）
		sampleHeadCut := headCut
		sampleTailCut := tailCut
		if headCutFromExcel > 0 {
			sampleHeadCut = headCutFromExcel
		}
		if tailCutFromExcel > 0 {
			sampleTailCut = tailCutFromExcel
		}
		totalLen := len(fullSeqFromExcel)
		if sampleHeadCut+sampleTailCut < totalLen {
			trimmedSeq := fullSeqFromExcel[sampleHeadCut : totalLen-sampleTailCut]
			acgtCounts := make(map[byte]int)
			for i := 0; i < len(trimmedSeq); i++ {
				b := trimmedSeq[i]
				if b == 'A' || b == 'C' || b == 'G' || b == 'T' {
					acgtCounts[b]++
				}
				// 根据需要也可统计N
			}
			sampleStats.RefACGTCounts = acgtCounts
			sampleStats.RefLengthAfterTrim = len(trimmedSeq)
		} else {
			fmt.Printf("  警告: 样本 %s 的切除长度超过序列全长\n", sampleName)
		}

		sampleStats.Unlock()
	}

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

		// 新增：N-mer 统计所需变量
		consecutiveMatch := 0 // 当前位置之前连续正确的碱基数（不包括当前）

		refPos := refStart
		readPos := 0
		readIsPerfect := true
		perfectUptoNow := true
		cigarOps := read.Cigar

		alignedBasesThisRead := 0
		// 消耗参考位置的操作：M, D, N, =, X
		// alignedBasesThisRead += length
		for ci, cigarOp := range cigarOps {
			op := cigarOp.Type()
			length := cigarOp.Len()

			switch op {
			case 0, 7, 8: // M, =, X
				alignedBasesThisRead += length
				for j := range length {
					currentPos := refPos + j + 1 // 1-based
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
					detail, exists := sampleStats.PositionStats[currentPos]
					if !exists {
						detail = &PositionDetail{}
						sampleStats.PositionStats[currentPos] = detail
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

					// 根据之前连续正确数判断是否满足 N 条件
					if consecutiveMatch >= nMerSize {
						// 获取该位置的 N-mer 统计对象
						posStats, ok := sampleStats.NMerStats[currentPos]
						if !ok {
							posStats = &PositionNMerStats{}
							sampleStats.NMerStats[currentPos] = posStats
						}
						posStats.NCorrect++ // 前 N 个碱基正确
						if !isMismatch {
							posStats.N1Correct++ // 前 N+1 个碱基正确
						}
					}

					// 更新连续匹配计数器
					if !isMismatch {
						consecutiveMatch++
					} else {
						consecutiveMatch = 0
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
				alignedBasesThisRead += length
				for j := range length {
					pos := refPos + j + 1
					sampleStats.Lock()
					detail, exists := sampleStats.PositionStats[pos]
					if !exists {
						detail = &PositionDetail{}
						sampleStats.PositionStats[pos] = detail
					}
					detail.Depth++
					detail.Deletion++
					if length == 1 {
						detail.Del1++
					}
					sampleStats.Unlock()
				}
				readIsPerfect = false
				perfectUptoNow = false

				// 缺失时不产生覆盖位置，但会打断连续性
				consecutiveMatch = 0

				refPos += length

			case 1: // I
				readPos += length
				readIsPerfect = false
				perfectUptoNow = false
				// 插入本身不占用参考位置，但影响前一个位置的统计（已在 M 操作中处理）
				// 插入打断连续性
				consecutiveMatch = 0
			case 4: // S
				readPos += length
				// 软剪接通常表示 read 开头或结尾，不影响连续匹配？但实际会打断，因为前面的碱基不参与参考序列
				// 保守重置
				consecutiveMatch = 0
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
		readInfo := analyzeReadDetailedInfo(read, mdStr, fullSeqFromExcel)

		// 统计突变信息
		mutationCount := len(readInfo.Mutations)
		sampleStats.Lock()
		sampleStats.SubstitutionCountDist[mutationCount]++
		sampleStats.AlignedBases += alignedBasesThisRead

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
			if len(readInfo.DeleteSub.Deletions) > 0 {
				// reads维度：包含缺失的reads数
				sampleStats.DeleteReads++
			}
			for _, del := range readInfo.DeleteSub.Deletions {
				// 缺失事件个数维度：每个缺失事件计数
				sampleStats.DeleteEventCount++

				// 缺失碱基个数维度：累加缺失碱基数
				sampleStats.DeleteBaseTotal += del.Length
				sampleStats.MutationBaseCounts["deletion"] += del.Length

				// 缺失长度分布
				length := del.Length
				if length > 3 {
					length = 4 // >3
				}
				sampleStats.DeleteLengthDist[length]++

				st := del.Subtype
				sampleStats.DeleteSubtypeEvents[st]++
				sampleStats.DeleteSubtypeBases[st] += del.Length
				deleteSubtypes[st] = true

				// 单碱基缺失统计
				if st == Del1 && len(del.Bases) == 1 {
					base := del.Bases[0]
					sampleStats.Del1BaseCounts[base]++

					// 缺失位置统计
					posKey := fmt.Sprintf("%d:%c", del.Position, base)
					sampleStats.DeletePositionCounts[posKey]++
				}

				// 新增：对于 Del3，记录前一个碱基和第一个缺失碱基
				if st == Del3 && fullSeqFromExcel != "" {
					// 缺失位置（1-based）
					pos := del.Position
					var prevBase byte
					var firstBase byte
					// 前一个碱基位置（注意：pos-1 必须在有效范围内）
					if pos-1 >= 1 && pos-1 <= len(fullSeqFromExcel) {
						prevBase = fullSeqFromExcel[pos-2] // 字符串索引从0开始
						sampleStats.Del3PrevBaseCounts[prevBase]++
					}
					// 第一个缺失碱基
					if len(del.Bases) > 0 {
						firstBase = del.Bases[0]
						sampleStats.Del3FirstBaseCounts[firstBase]++
					}
					combKey := fmt.Sprintf("%c>%c", prevBase, firstBase)
					sampleStats.Del3PrevFirstCombCounts[combKey]++
				}
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

	// 累加 ACGT 统计
	if fullSeqFromExcel != "" {
		sampleStats.RLock()
		stats.TotalRefACGTCounts['A'] += sampleStats.RefACGTCounts['A']
		stats.TotalRefACGTCounts['C'] += sampleStats.RefACGTCounts['C']
		stats.TotalRefACGTCounts['G'] += sampleStats.RefACGTCounts['G']
		stats.TotalRefACGTCounts['T'] += sampleStats.RefACGTCounts['T']
		stats.TotalRefLengthAfterTrim += sampleStats.RefLengthAfterTrim
		sampleStats.RUnlock()
	}

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

	// 合并 Del3 相邻碱基分布
	for base, cnt := range sampleStats.Del3PrevBaseCounts {
		stats.TotalDel3PrevBaseCounts[base] += cnt
	}
	for base, cnt := range sampleStats.Del3FirstBaseCounts {
		stats.TotalDel3FirstBaseCounts[base] += cnt
	}
	for key, cnt := range sampleStats.Del3PrevFirstCombCounts {
		stats.TotalDel3PrevFirstCombCounts[key] += cnt
	}

	stats.TotalGoodAlignedReads += sampleStats.GoodAlignedReads
	// 累加 RefLength * GoodAlignedReads
	stats.TotalRefLengthGoodAligned += (sampleStats.RefLength - sampleStats.HeadCut - sampleStats.TailCut) * sampleStats.GoodAlignedReads
	stats.TotalAlignedBases += sampleStats.AlignedBases

	sampleStats.RUnlock()
	stats.Unlock()
	/*
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
	*/
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
			fullSeq := ""
			if fullSeqs != nil {
				fullSeq = fullSeqs[sampleName]
				refLen = len(fullSeq)
				headCutFromExcel = headCuts[sampleName]
				tailCutFromExcel = tailCuts[sampleName]
			}

			// 处理BAM文件
			if err := processBAMFile(path, sampleName, stats,
				refLen, headCutFromExcel, tailCutFromExcel, fullSeq); err != nil {
				fmt.Printf("处理文件 %s 失败: %v\n", path, err)
			}
		}(bamPath)
	}

	// 等待所有goroutine完成
	wg.Wait()

	return stats
}

// findBAMFiles 查找所有BAM文件，并尝试补充参考序列
func findBAMFiles(inputDir string, fullSeqs map[string]string) ([]string, error) {
	var bamFiles []string

	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		for _, sampleName := range sampleOrder {
			sortBam := filepath.Join(inputDir, "samples", sampleName, sampleName+".sorted.bam")
			filterBam := filepath.Join(inputDir, sampleName, sampleName+".filter.bam")
			var bamPath string
			if _, err := os.Stat(sortBam); err == nil {
				bamPath = sortBam
			} else if _, err := os.Stat(filterBam); err == nil {
				bamPath = filterBam
			} else {
				fmt.Printf("警告: 找不到样本 %s 的BAM文件: %s 或 %s\n", sampleName, sortBam, filterBam)
				continue
			}
			bamFiles = append(bamFiles, bamPath)

			// 若 fullSeqs 中尚无该样本的参考序列，尝试查找 .ref.fa 文件
			if _, ok := fullSeqs[sampleName]; !ok || fullSeqs[sampleName] == "" {
				refCandidates := []string{
					filepath.Join(inputDir, "samples", sampleName, sampleName+".ref.fa"),
					filepath.Join(inputDir, sampleName, sampleName+".ref.fa"),
				}
				for _, refPath := range refCandidates {
					if _, err := os.Stat(refPath); err == nil {
						seq, err := readRefFasta(refPath)
						if err != nil {
							fmt.Printf("警告: 读取参考文件 %s 失败: %v\n", refPath, err)
						} else {
							fullSeqs[sampleName] = seq
							fmt.Printf("从参考文件 %s 加载序列，长度 %d\n", refPath, len(seq))
						}
						break
					}
				}
			}
		}
	} else {
		// 原来的方法：递归查找所有BAM文件
		err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && (strings.HasSuffix(path, ".sorted.bam") || strings.HasSuffix(path, ".filter.bam")) {
				bamFiles = append(bamFiles, path)

				// ===== 新增：尝试补充参考序列 =====
				sampleName := filepath.Base(filepath.Dir(path))
				if _, ok := fullSeqs[sampleName]; !ok || fullSeqs[sampleName] == "" {
					// 构造可能的参考文件路径
					refCandidates := []string{
						filepath.Join(filepath.Dir(path), sampleName+".ref.fa"),
						filepath.Join(filepath.Dir(path), "ref.fa"),
					}
					for _, refPath := range refCandidates {
						if _, err := os.Stat(refPath); err == nil {
							seq, err := readRefFasta(refPath)
							if err != nil {
								fmt.Printf("警告: 读取参考文件 %s 失败: %v\n", refPath, err)
							} else {
								fullSeqs[sampleName] = seq
								fmt.Printf("从参考文件 %s 加载序列，长度 %d\n", refPath, len(seq))
							}
							break
						}
					}
				}
				// ===== 新增结束 =====
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

// readRefFasta 读取FASTA文件，返回第一条序列的序列字符串（忽略标题行）
func readRefFasta(fastaPath string) (string, error) {
	file, err := os.Open(fastaPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var seq strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">") {
			continue
		}
		seq.WriteString(strings.TrimSpace(line))
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return seq.String(), nil
}
