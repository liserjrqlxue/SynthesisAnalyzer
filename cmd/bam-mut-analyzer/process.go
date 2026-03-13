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

// processBAMFile 处理单个BAM文件 - 优化版本
func processBAMFile(bamPath, sampleName string, stats *MutationStats, refLenFromExcel, headCutFromExcel, tailCutFromExcel int, fullSeqFromExcel string) error {
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

	sampleStats := stats.getOrCreateSampleStats(sampleName)

	// 预计算参考序列信息
	if fullSeqFromExcel != "" {
		sampleStats.Lock()
		sampleStats.RefSeqFull = fullSeqFromExcel

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
			acgtCounts := make(map[byte]int, 4)
			for i := range trimmedSeq {
				b := trimmedSeq[i]
				if b == 'A' || b == 'C' || b == 'G' || b == 'T' {
					acgtCounts[b]++
				}
			}
			sampleStats.RefACGTCounts = acgtCounts
			sampleStats.RefLengthAfterTrim = len(trimmedSeq)
		} else {
			fmt.Printf("  警告: 样本 %s 的切除长度超过序列全长\n", sampleName)
		}
		sampleStats.Unlock()
	}

	// 获取参考序列长度
	refSeqLen := 0
	for _, ref := range br.Header().Refs() {
		if ref.Name() == sampleName {
			refSeqLen = ref.Len()
			break
		}
	}
	if refSeqLen == 0 && refLenFromExcel > 0 {
		refSeqLen = refLenFromExcel
	}
	if refSeqLen == 0 {
		fmt.Printf("  警告: 无法获取样本 %s 的参考序列长度\n", sampleName)
	} else if refSeqLen != refLenFromExcel && refLenFromExcel > 0 {
		fmt.Printf("  警告: 样本 %s 的参考序列长度冲突：[%d vs. %d]\n", sampleName, refSeqLen, refLenFromExcel)
	}

	// 预计算头尾切除参数
	sampleHeadCut := headCut
	sampleTailCut := tailCut
	if headCutFromExcel > 0 {
		sampleHeadCut = headCutFromExcel
	}
	if tailCutFromExcel > 0 {
		sampleTailCut = tailCutFromExcel
	}

	// 使用局部变量收集统计，避免频繁锁操作
	type localPosStats struct {
		detail              PositionDetail
		nMerStats           *PositionNMerStats
		perfectCountUpdated bool
	}

	localPosMap := make(map[int]*localPosStats)
	localReadStats := &SampleStats{}
	localReadStats.SubstitutionCountDist = make(map[int]int)
	localReadStats.InsertLengthDist = make(map[int]int)
	localReadStats.DeleteLengthDist = make(map[int]int)
	localReadStats.InsertBaseCounts = make(map[string]int)
	localReadStats.DeletePositionCounts = make(map[string]int)
	localReadStats.MutationBaseCounts = make(map[string]int)
	localReadStats.Mutations = make(map[string]int)
	localReadStats.PositionMutations = make(map[string]int)
	localReadStats.PositionDetails = make(map[string]map[string]int)
	localReadStats.ReadTypeCounts = make(map[ReadType]int)

	// 细分类统计初始化
	localReadStats.DeleteSubtypeReads = make(map[DeletionSubtype]int)
	localReadStats.DeleteSubtypeEvents = make(map[DeletionSubtype]int)
	localReadStats.DeleteSubtypeBases = make(map[DeletionSubtype]int)
	localReadStats.InsertSubtypeReads = make(map[InsertionSubtype]int)
	localReadStats.InsertSubtypeEvents = make(map[InsertionSubtype]int)
	localReadStats.InsertSubtypeBases = make(map[InsertionSubtype]int)
	localReadStats.SubstitutionSubtypeReads = make(map[SubstitutionSubtype]int)
	localReadStats.SubstitutionSubtypeEvents = make(map[SubstitutionSubtype]int)
	localReadStats.SubstitutionSubtypeBases = make(map[SubstitutionSubtype]int)
	localReadStats.SubtypeCombinationCounts = make(map[string]int)
	localReadStats.Del1BaseCounts = make(map[byte]int)
	localReadStats.Del3PrevBaseCounts = make(map[byte]int)
	localReadStats.Del3FirstBaseCounts = make(map[byte]int)
	localReadStats.Del3PrevFirstCombCounts = make(map[string]int)
	localReadStats.MutationList = make([]Mutation, 0, 64)

	// 预分配buffer减少GC
	var totalReads, alignedReads int
	var readsWithMutations, totalMutations int
	alignedBasesThisRead := 0

	// 遍历所有记录
	for {
		read, err := br.Read()
		if err != nil {
			break
		}

		totalReads++

		// 跳过未比对记录
		if read.Flags&sam.Unmapped != 0 {
			continue
		}
		alignedReads++
		alignedBasesThisRead = 0

		// 获取MD字符串并缓存解析结果
		mdTag, hasMD := read.Tag([]byte{'M', 'D'})
		var mdStr string
		if hasMD {
			mdStr = mdTag.String()
			if len(mdStr) > 5 && mdStr[4] == ':' {
				mdStr = mdStr[5:]
			} else {
				mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
			}
		}

		refStart := int(read.Pos)
		mdMap := parseMDToMap(mdStr, refStart)

		// 预计算位置范围，减少边界检查
		consecutiveMatch := 0
		refPos := refStart
		readPos := 0
		readIsPerfect := true
		perfectUptoNow := true
		cigarOps := read.Cigar
		cigarLen := len(cigarOps)

		// 第一次遍历：收集位置统计和突变信息
		var mutationList []Mutation
		var insertSubtypes = make(map[InsertionSubtype]bool)
		var deleteSubtypes = make(map[DeletionSubtype]bool)
		var substSubtypes = make(map[SubstitutionSubtype]bool)
		var deleteList []DeletionInfo

		for ci := range cigarLen {
			cigarOp := cigarOps[ci]
			op := cigarOp.Type()
			length := cigarOp.Len()

			switch op {
			case 0, 7, 8: // M, =, X
				alignedBasesThisRead += length
				isMismatchOp := (op == 8)

				for j := range length {
					currentPos := refPos + j + 1
					isMismatch := isMismatchOp
					if !isMismatch && hasMD {
						_, isMismatch = mdMap[refPos+j]
					}

					// 检查下一个操作是否为插入
					hasInsertAfter := (j == length-1 && ci+1 < cigarLen && cigarOps[ci+1].Type() == 1)

					// 获取或创建位置统计
					posStats, exists := localPosMap[currentPos]
					if !exists {
						posStats = &localPosStats{}
						localPosMap[currentPos] = posStats
					}
					posStats.detail.Depth++

					if perfectUptoNow && !isMismatch && !hasInsertAfter {
						posStats.detail.PerfectUptoPosCount++
					}

					if hasInsertAfter {
						if isMismatch {
							posStats.detail.MismatchWithIns++
						} else {
							posStats.detail.MatchWithIns++
						}
						posStats.detail.Insertion++
					} else {
						if isMismatch {
							posStats.detail.MismatchPure++
						} else {
							posStats.detail.MatchPure++
						}
					}

					// N-mer统计
					if consecutiveMatch >= nMerSize {
						if posStats.nMerStats == nil {
							posStats.nMerStats = &PositionNMerStats{}
						}
						posStats.nMerStats.NCorrect++
						if !isMismatch {
							posStats.nMerStats.N1Correct++
						}
					}

					if !isMismatch {
						consecutiveMatch++
					} else {
						consecutiveMatch = 0
					}

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
					posStats, exists := localPosMap[pos]
					if !exists {
						posStats = &localPosStats{}
						localPosMap[pos] = posStats
					}
					posStats.detail.Depth++
					posStats.detail.Deletion++
					if length == 1 {
						posStats.detail.Del1++
					}
				}
				readIsPerfect = false
				perfectUptoNow = false
				consecutiveMatch = 0
				refPos += length

			case 1: // I
				readPos += length
				readIsPerfect = false
				perfectUptoNow = false
				consecutiveMatch = 0
			case 4: // S
				readPos += length
				consecutiveMatch = 0
			}
		}

		// 第二次遍历：如果是完美read，收集需要更新PerfectReadsCount的位置
		if readIsPerfect {
			refPos = refStart
			for ci := range cigarLen {
				cigarOp := cigarOps[ci]
				op := cigarOp.Type()
				length := cigarOp.Len()

				switch op {
				case 0, 7, 8:
					for j := range length {
						pos := refPos + j + 1
						posStats, exists := localPosMap[pos]
						if exists {
							posStats.perfectCountUpdated = true
							posStats.detail.PerfectReadsCount++
						}
					}
					refPos += length
				case 2, 3:
					refPos += length
				}
			}
		}

		// 分析详细的read类型
		readInfo := analyzeReadDetailedInfo(read, mdMap, mdStr, fullSeqFromExcel)
		localReadStats.ReadTypeCounts[readInfo.MainType]++

		mutationCount := len(readInfo.Mutations)
		localReadStats.SubstitutionCountDist[mutationCount]++
		localReadStats.AlignedBases += alignedBasesThisRead

		// 跳过非良好比对
		if mutationCount > maxSubstitutions {
			continue
		}
		localReadStats.GoodAlignedReads++

		// 统计插入信息
		if readInfo.InsertSub != nil && len(readInfo.InsertSub.Insertions) > 0 {
			localReadStats.InsertReads++

			for _, insertion := range readInfo.InsertSub.Insertions {
				localReadStats.InsertEventCount++
				localReadStats.InsertBaseTotal += insertion.Length
				localReadStats.MutationBaseCounts["insertion"] += insertion.Length

				length := insertion.Length
				if length > 3 {
					length = 4
				}
				localReadStats.InsertLengthDist[length]++

				if insertion.Bases != "" {
					localReadStats.InsertBaseCounts[insertion.Bases]++
				}

				st := insertion.Subtype
				localReadStats.InsertSubtypeEvents[st]++
				localReadStats.InsertSubtypeBases[st] += insertion.Length
				insertSubtypes[st] = true
			}
			// 修复：InsertSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range insertSubtypes {
				localReadStats.InsertSubtypeReads[st]++
			}
		}

		// 统计突变信息
		if mutationCount > 0 {
			readsWithMutations++
			totalMutations += mutationCount
			localReadStats.SubstitutionReads++
			localReadStats.SubstitutionEventCount += mutationCount
			localReadStats.SubstitutionBaseTotal += mutationCount
			localReadStats.MutationBaseCounts["substitution"] += mutationCount

			for _, mut := range readInfo.Mutations {
				posKey := strconv.Itoa(mut.Position)
				mutKey := mut.Ref + ">" + mut.Alt
				posMutKey := posKey + "_" + mutKey

				if _, ok := localReadStats.PositionDetails[posKey]; !ok {
					localReadStats.PositionDetails[posKey] = make(map[string]int)
				}
				localReadStats.PositionDetails[posKey][mutKey]++
				localReadStats.Mutations[mutKey]++
				localReadStats.PositionMutations[posMutKey]++
				mutationList = append(mutationList, mut)
			}
		}

		localReadStats.MutationList = append(localReadStats.MutationList, mutationList...)

		// 缺失细分类统计
		if readInfo.DeleteSub != nil {
			if len(readInfo.DeleteSub.Deletions) > 0 {
				localReadStats.DeleteReads++
			}
			for _, del := range readInfo.DeleteSub.Deletions {
				localReadStats.DeleteEventCount++
				localReadStats.DeleteBaseTotal += del.Length
				localReadStats.MutationBaseCounts["deletion"] += del.Length

				length := del.Length
				if length > 3 {
					length = 4
				}
				localReadStats.DeleteLengthDist[length]++

				st := del.Subtype
				localReadStats.DeleteSubtypeEvents[st]++
				localReadStats.DeleteSubtypeBases[st] += del.Length
				deleteSubtypes[st] = true
				deleteList = append(deleteList, del)

				if st == Del1 && len(del.Bases) == 1 {
					base := del.Bases[0]
					localReadStats.Del1BaseCounts[base]++
					posKey := fmt.Sprintf("%d:%c", del.Position, base)
					localReadStats.DeletePositionCounts[posKey]++
				}

				// Del3 统计
				if st == Del3 && fullSeqFromExcel != "" {
					pos := del.Position
					var prevBase byte
					var firstBase byte
					if pos-1 >= 1 && pos-1 <= len(fullSeqFromExcel) {
						prevBase = fullSeqFromExcel[pos-2]
						localReadStats.Del3PrevBaseCounts[prevBase]++
					}
					if len(del.Bases) > 0 {
						firstBase = del.Bases[0]
						localReadStats.Del3FirstBaseCounts[firstBase]++
					}
					combKey := fmt.Sprintf("%c>%c", prevBase, firstBase)
					localReadStats.Del3PrevFirstCombCounts[combKey]++
				}
			}
			// 修复：DeleteSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range deleteSubtypes {
				localReadStats.DeleteSubtypeReads[st]++
			}
		}

		// 替换细分类统计
		if len(readInfo.Mutations) > 0 {
			for _, sub := range readInfo.Mutations {
				st := sub.Subtype
				localReadStats.SubstitutionSubtypeEvents[st]++
				localReadStats.SubstitutionSubtypeBases[st]++
				substSubtypes[st] = true
			}
			// 修复：SubstitutionSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range substSubtypes {
				localReadStats.SubstitutionSubtypeReads[st]++
			}
		}

		// 细分类组合统计
		if len(insertSubtypes) > 0 || len(deleteSubtypes) > 0 || len(substSubtypes) > 0 {
			combKey := buildSubtypeCombinationKey(insertSubtypes, deleteSubtypes, substSubtypes)
			localReadStats.SubtypeCombinationCounts[combKey]++
		}
	}

	// 更新样本统计 - 一次性加锁合并
	sampleStats.Lock()
	sampleStats.ReadCounts = totalReads
	sampleStats.AlignedReads = alignedReads
	sampleStats.ReadsWithMutations = readsWithMutations
	sampleStats.RefLength = refSeqLen
	sampleStats.HeadCut = sampleHeadCut
	sampleStats.TailCut = sampleTailCut
	for _, count := range localReadStats.Mutations {
		sampleStats.TotalMutations += count
	}

	// 合并read类型统计
	for rt, count := range localReadStats.ReadTypeCounts {
		sampleStats.ReadTypeCounts[rt] += count
	}

	// 合并突变统计
	for mutKey, count := range localReadStats.Mutations {
		sampleStats.Mutations[mutKey] += count
	}
	for posMutKey, count := range localReadStats.PositionMutations {
		sampleStats.PositionMutations[posMutKey] += count
	}
	for posKey, mutMap := range localReadStats.PositionDetails {
		if sampleStats.PositionDetails[posKey] == nil {
			sampleStats.PositionDetails[posKey] = make(map[string]int)
		}
		for mutKey, count := range mutMap {
			sampleStats.PositionDetails[posKey][mutKey] += count
		}
	}

	// 合并插入统计
	sampleStats.InsertReads += localReadStats.InsertReads
	sampleStats.InsertEventCount += localReadStats.InsertEventCount
	sampleStats.InsertBaseTotal += localReadStats.InsertBaseTotal
	for length, count := range localReadStats.InsertLengthDist {
		sampleStats.InsertLengthDist[length] += count
	}
	for seq, count := range localReadStats.InsertBaseCounts {
		sampleStats.InsertBaseCounts[seq] += count
	}

	// 合并缺失统计
	sampleStats.DeleteReads += localReadStats.DeleteReads
	sampleStats.DeleteEventCount += localReadStats.DeleteEventCount
	sampleStats.DeleteBaseTotal += localReadStats.DeleteBaseTotal
	for length, count := range localReadStats.DeleteLengthDist {
		sampleStats.DeleteLengthDist[length] += count
	}
	for posKey, count := range localReadStats.DeletePositionCounts {
		sampleStats.DeletePositionCounts[posKey] += count
	}
	for base, count := range localReadStats.Del1BaseCounts {
		sampleStats.Del1BaseCounts[base] += count
	}

	// 合并替换统计
	sampleStats.SubstitutionReads += localReadStats.SubstitutionReads
	sampleStats.SubstitutionEventCount += localReadStats.SubstitutionEventCount
	sampleStats.SubstitutionBaseTotal += localReadStats.SubstitutionBaseTotal

	// 合并碱基维度统计
	for key, count := range localReadStats.MutationBaseCounts {
		sampleStats.MutationBaseCounts[key] += count
	}

	// 合并细分类统计
	for st, cnt := range localReadStats.DeleteSubtypeReads {
		sampleStats.DeleteSubtypeReads[st] += cnt
	}
	for st, cnt := range localReadStats.DeleteSubtypeEvents {
		sampleStats.DeleteSubtypeEvents[st] += cnt
	}
	for st, cnt := range localReadStats.DeleteSubtypeBases {
		sampleStats.DeleteSubtypeBases[st] += cnt
	}

	for st, cnt := range localReadStats.InsertSubtypeReads {
		sampleStats.InsertSubtypeReads[st] += cnt
	}
	for st, cnt := range localReadStats.InsertSubtypeEvents {
		sampleStats.InsertSubtypeEvents[st] += cnt
	}
	for st, cnt := range localReadStats.InsertSubtypeBases {
		sampleStats.InsertSubtypeBases[st] += cnt
	}

	for st, cnt := range localReadStats.SubstitutionSubtypeReads {
		sampleStats.SubstitutionSubtypeReads[st] += cnt
	}
	for st, cnt := range localReadStats.SubstitutionSubtypeEvents {
		sampleStats.SubstitutionSubtypeEvents[st] += cnt
	}
	for st, cnt := range localReadStats.SubstitutionSubtypeBases {
		sampleStats.SubstitutionSubtypeBases[st] += cnt
	}

	for key, cnt := range localReadStats.SubtypeCombinationCounts {
		sampleStats.SubtypeCombinationCounts[key] += cnt
	}

	// 合并替换个数分布
	for count, num := range localReadStats.SubstitutionCountDist {
		sampleStats.SubstitutionCountDist[count] += num
	}

	// 合并Del3统计
	for base, count := range localReadStats.Del3PrevBaseCounts {
		sampleStats.Del3PrevBaseCounts[base] += count
	}
	for base, count := range localReadStats.Del3FirstBaseCounts {
		sampleStats.Del3FirstBaseCounts[base] += count
	}
	for key, count := range localReadStats.Del3PrevFirstCombCounts {
		sampleStats.Del3PrevFirstCombCounts[key] += count
	}

	// 合并其他统计
	sampleStats.GoodAlignedReads += localReadStats.GoodAlignedReads
	sampleStats.AlignedBases += localReadStats.AlignedBases

	// 合并位置统计
	if sampleStats.PositionStats == nil {
		sampleStats.PositionStats = make(map[int]*PositionDetail, len(localPosMap))
	}
	if sampleStats.NMerStats == nil {
		sampleStats.NMerStats = make(map[int]*PositionNMerStats, len(localPosMap))
	}

	for pos, localStats := range localPosMap {
		detail, exists := sampleStats.PositionStats[pos]
		if !exists {
			detail = &PositionDetail{}
			sampleStats.PositionStats[pos] = detail
		}
		detail.Depth += localStats.detail.Depth
		detail.MatchPure += localStats.detail.MatchPure
		detail.MatchWithIns += localStats.detail.MatchWithIns
		detail.MismatchPure += localStats.detail.MismatchPure
		detail.MismatchWithIns += localStats.detail.MismatchWithIns
		detail.Insertion += localStats.detail.Insertion
		detail.Deletion += localStats.detail.Deletion
		detail.Del1 += localStats.detail.Del1
		detail.PerfectReadsCount += localStats.detail.PerfectReadsCount
		detail.PerfectUptoPosCount += localStats.detail.PerfectUptoPosCount

		if localStats.nMerStats != nil {
			nMer, exists := sampleStats.NMerStats[pos]
			if !exists {
				nMer = &PositionNMerStats{}
				sampleStats.NMerStats[pos] = nMer
			}
			nMer.NCorrect += localStats.nMerStats.NCorrect
			nMer.N1Correct += localStats.nMerStats.N1Correct
		}
	}
	sampleStats.Unlock()

	// 平滑连续碱基缺失分配
	sampleStats.NormalizeDeletionsByContinuousBases()

	// 更新总统计
	stats.Lock()
	stats.TotalReadCount += totalReads
	stats.TotalAlignedReads += alignedReads
	stats.TotalReadsWithMuts += readsWithMutations

	// 合并 ACGT 统计
	if fullSeqFromExcel != "" {
		sampleStats.RLock()
		stats.TotalRefACGTCounts['A'] += sampleStats.RefACGTCounts['A']
		stats.TotalRefACGTCounts['C'] += sampleStats.RefACGTCounts['C']
		stats.TotalRefACGTCounts['G'] += sampleStats.RefACGTCounts['G']
		stats.TotalRefACGTCounts['T'] += sampleStats.RefACGTCounts['T']
		stats.TotalRefLengthAfterTrim += sampleStats.RefLengthAfterTrim
		sampleStats.RUnlock()
	}

	// 合并样本统计到总统计 - 使用批量操作
	sampleStats.RLock()

	// 合并read类型统计
	for rt, count := range sampleStats.ReadTypeCounts {
		stats.TotalReadTypeCounts[rt] += count
	}

	// 合并突变统计
	for mutKey, count := range sampleStats.Mutations {
		stats.TotalMutations[mutKey] += count
	}

	// 合并插入统计
	stats.TotalInsertReads += sampleStats.InsertReads
	stats.TotalInsertEventCount += sampleStats.InsertEventCount
	stats.TotalInsertBaseTotal += sampleStats.InsertBaseTotal
	for length, count := range sampleStats.InsertLengthDist {
		stats.TotalInsertLengthDist[length] += count
	}
	for seq, count := range sampleStats.InsertBaseCounts {
		stats.TotalInsertBaseCounts[seq] += count
	}

	// 合并缺失统计
	stats.TotalDeleteReads += sampleStats.DeleteReads
	stats.TotalDeleteEventCount += sampleStats.DeleteEventCount
	stats.TotalDeleteBaseTotal += sampleStats.DeleteBaseTotal
	for length, count := range sampleStats.DeleteLengthDist {
		stats.TotalDeleteLengthDist[length] += count
	}
	for posKey, count := range sampleStats.DeletePositionCounts {
		stats.TotalDeletePositionCounts[posKey] += count
	}
	// 合并Del1碱基统计
	for base, count := range sampleStats.Del1BaseCounts {
		stats.TotalDel1BaseCounts[base] += count
	}

	// 合并替换统计
	stats.TotalSubstitutionReads += sampleStats.SubstitutionReads
	stats.TotalSubstitutionEventCount += sampleStats.SubstitutionEventCount
	stats.TotalSubstitutionBaseTotal += sampleStats.SubstitutionBaseTotal

	// 合并碱基维度统计 - 这些已经在上面的统计中包含了，不需要重复计算
	/*
		for key, count := range sampleStats.MutationBaseCounts {
			if key == "insertion" {
				stats.TotalInsertBaseTotal += count
			} else if key == "deletion" {
				stats.TotalDeleteBaseTotal += count
			} else if key == "substitution" {
				stats.TotalSubstitutionBaseTotal += count
			}
		}
	*/

	// 合并细分类统计
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

	for key, cnt := range sampleStats.SubtypeCombinationCounts {
		stats.TotalSubtypeCombinationCounts[key] += cnt
	}

	// 合并替换个数分布
	for count, n := range sampleStats.SubstitutionCountDist {
		stats.TotalSubstitutionCountDist[count] += n
	}

	// 合并Del3统计
	for base, cnt := range sampleStats.Del3PrevBaseCounts {
		stats.TotalDel3PrevBaseCounts[base] += cnt
	}
	for base, cnt := range sampleStats.Del3FirstBaseCounts {
		stats.TotalDel3FirstBaseCounts[base] += cnt
	}
	for key, cnt := range sampleStats.Del3PrevFirstCombCounts {
		stats.TotalDel3PrevFirstCombCounts[key] += cnt
	}

	// 合并其他统计
	stats.TotalGoodAlignedReads += sampleStats.GoodAlignedReads
	trimmedLen := refSeqLen - sampleHeadCut - sampleTailCut
	if trimmedLen > 0 {
		stats.TotalRefLengthGoodAligned += trimmedLen * sampleStats.GoodAlignedReads
	}
	stats.TotalAlignedBases += sampleStats.AlignedBases

	sampleStats.RUnlock()
	stats.Unlock()

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
							// fmt.Printf("从参考文件 %s 加载序列，长度 %d\n", refPath, len(seq))
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
