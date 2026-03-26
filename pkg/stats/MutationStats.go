package stats

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
	"golang.org/x/sync/errgroup"
)

// MutationStats 存储突变统计 - 添加新字段
type MutationStats struct {
	sync.RWMutex
	SampleInfo  *BatchInfo              // 样本信息指针
	Samples     map[string]*SampleStats // sample -> 样本统计
	SampleNames []string                // 样本名顺序

	TotalMutations      map[string]int   // mutation -> total count
	TotalReadTypeCounts map[ReadType]int // 所有样本的read type统计

	TotalDeleteLengthDist     map[int]int    // 所有样本的缺失长度分布
	TotalMaxDeleteLengthDist  map[int]int    // 所有样本的最长缺失长度分布
	TotalDeleteBaseCounts     map[byte]int   // 所有样本的缺失碱基统计
	TotalDeletePositionCounts map[string]int // 所有样本的缺失位置统计

	TotalInsertLengthDist map[int]int    // 所有样本的插入长度分布
	TotalInsertBaseCounts map[string]int // 所有样本的插入序列统计

	TotalReadCount     int // 所有样本的总reads数
	TotalAlignedReads  int // 所有样本的比对reads数
	TotalReadsWithMuts int // 所有样本的包含突变reads数的和
	// 新增：汇总的reads维度变异统计
	TotalInsertReads       int
	TotalDeleteReads       int
	TotalSubstitutionReads int

	// 新增：汇总的变异事件个数维度
	TotalInsertEventCount       int
	TotalDeleteEventCount       int
	TotalSubstitutionEventCount int

	// 新增：汇总的碱基个数维度
	TotalInsertBaseTotal       int
	TotalDeleteBaseTotal       int
	TotalSubstitutionBaseTotal int

	// 新增：总细分类统计
	TotalDeleteSubtypeReads        map[DeletionSubtype]int
	TotalDeleteSubtypeEvents       map[DeletionSubtype]int
	TotalDeleteSubtypeBases        map[DeletionSubtype]int
	TotalInsertSubtypeReads        map[InsertionSubtype]int
	TotalInsertSubtypeEvents       map[InsertionSubtype]int
	TotalInsertSubtypeBases        map[InsertionSubtype]int
	TotalSubstitutionSubtypeReads  map[SubstitutionSubtype]int
	TotalSubstitutionSubtypeEvents map[SubstitutionSubtype]int
	TotalSubstitutionSubtypeBases  map[SubstitutionSubtype]int
	TotalSubtypeCombinationCounts  map[string]int // 总体组合统计

	TotalSubstitutionCountDist map[int]int // 总体分布
	TotalGoodAlignedReads      int         // 总体比对良好reads数
	// 新增：总Del1缺失碱基分布
	TotalDel1BaseCounts map[byte]int
	// 新增：RefLength * GoodAlignedReads 的累加（用于比例计算）
	TotalRefLengthGoodAligned int // 所有样本的 RefLength * GoodAlignedReads 之和
	TotalAlignedBases         int // 所有样本的 AlignedBases 之和

	TotalRefACGTCounts      map[byte]int // 所有样本切除后参考中ACGT计数之和
	TotalRefLengthAfterTrim int          // 所有样本切除后参考长度之和

	// 新增：总 Del3 缺失前一个碱基分布
	TotalDel3PrevBaseCounts map[byte]int
	// 新增：总 Del3 缺失第一个碱基分布
	TotalDel3FirstBaseCounts     map[byte]int
	TotalDel3PrevFirstCombCounts map[string]int
}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	stats := &MutationStats{
		Samples:                  make(map[string]*SampleStats),
		TotalMutations:           make(map[string]int),
		TotalReadTypeCounts:      make(map[ReadType]int),
		TotalInsertLengthDist:    make(map[int]int),
		TotalDeleteLengthDist:    make(map[int]int),
		TotalMaxDeleteLengthDist: make(map[int]int),

		TotalDeleteBaseCounts: make(map[byte]int),
		TotalInsertBaseCounts: make(map[string]int),

		TotalDeletePositionCounts: make(map[string]int),

		TotalDeleteSubtypeReads:        make(map[DeletionSubtype]int),
		TotalDeleteSubtypeEvents:       make(map[DeletionSubtype]int),
		TotalDeleteSubtypeBases:        make(map[DeletionSubtype]int),
		TotalInsertSubtypeReads:        make(map[InsertionSubtype]int),
		TotalInsertSubtypeEvents:       make(map[InsertionSubtype]int),
		TotalInsertSubtypeBases:        make(map[InsertionSubtype]int),
		TotalSubstitutionSubtypeReads:  make(map[SubstitutionSubtype]int),
		TotalSubstitutionSubtypeEvents: make(map[SubstitutionSubtype]int),
		TotalSubstitutionSubtypeBases:  make(map[SubstitutionSubtype]int),
		TotalSubtypeCombinationCounts:  make(map[string]int),

		TotalSubstitutionCountDist:   make(map[int]int),
		TotalDel1BaseCounts:          make(map[byte]int),
		TotalRefACGTCounts:           make(map[byte]int),
		TotalDel3PrevBaseCounts:      make(map[byte]int),
		TotalDel3FirstBaseCounts:     make(map[byte]int),
		TotalDel3PrevFirstCombCounts: make(map[string]int),
	}
	return stats
}

// GetOrCreateSampleStats 获取或创建样本统计对象
func (stats *MutationStats) GetOrCreateSampleStats(sampleName string) *SampleStats {
	stats.Lock()
	defer stats.Unlock()

	if sampleStats, ok := stats.Samples[sampleName]; ok {
		return sampleStats
	}

	sampleStats := NewSampleStats()
	stats.Samples[sampleName] = sampleStats
	stats.SampleNames = append(stats.SampleNames, sampleName)
	return sampleStats
}

// 辅助函数：获取排序后的样本名列表
func (stats *MutationStats) SortSampleNames() {
	var (
		names       = stats.SampleNames
		sampleOrder = stats.SampleInfo.Order
	)
	if len(sampleOrder) > 0 {
		// 按 Excel 顺序排序
		orderMap := make(map[string]int)
		for i, n := range sampleOrder {
			orderMap[n] = i
		}
		sort.Slice(names, func(i, j int) bool {
			// 如果两个都在 sampleOrder 中，按顺序比较；否则按字母序
			oi, ei := orderMap[names[i]]
			oj, ej := orderMap[names[j]]
			if ei && ej {
				return oi < oj
			}
			if ei {
				return true
			}
			if ej {
				return false
			}
			return names[i] < names[j]
		})
	} else {
		sort.Strings(names)
	}
	stats.SampleNames = names
}

// ProcessBAMFile 处理单个BAM文件 - 优化版本（支持取消）
func (stats *MutationStats) ProcessBAMFile(ctx context.Context, sample *Sample) error {
	f, err := os.Open(sample.BamFile)
	if err != nil {
		return fmt.Errorf("打开BAM文件失败: %v", err)
	}
	defer f.Close()

	br, err := bam.NewReader(f, 0)
	if err != nil {
		return fmt.Errorf("创建BAM读取器失败: %v", err)
	}
	defer br.Close()

	// 初始化样本统计
	sampleStats := stats.GetOrCreateSampleStats(sample.Name)

	// 预计算参考序列信息
	if sample.FullSeqs != "" {
		sampleStats.RefSeqFull = sample.FullSeqs

		sampleHeadCut := sample.HeadCuts
		sampleTailCut := sample.TailCuts
		totalLen := len(sample.FullSeqs)
		if sampleHeadCut+sampleTailCut < totalLen {
			trimmedSeq := sample.FullSeqs[sampleHeadCut : totalLen-sampleTailCut]
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
			fmt.Printf("  警告: 样本 %s 的切除长度超过序列全长\n", sample.Name)
		}
	}

	// 获取参考序列长度
	refSeqLen := 0
	for _, ref := range br.Header().Refs() {
		if ref.Name() == sample.Name {
			refSeqLen = ref.Len()
			break
		}
	}
	if refSeqLen == 0 {
		refSeqLen = len(sample.FullSeqs)
	}
	if refSeqLen == 0 {
		fmt.Printf("  警告: 无法获取样本 %s 的参考序列长度\n", sample.Name)
	} else if refSeqLen != len(sample.FullSeqs) {
		fmt.Printf("  警告: 样本 %s 的参考序列长度冲突：[%d vs. %d]\n", sample.Name, refSeqLen, len(sample.FullSeqs))
	}

	// 预计算头尾切除参数
	sampleHeadCut := sample.HeadCuts
	sampleTailCut := sample.TailCuts

	// 基于参考序列长度预定义PositionStats
	if refSeqLen > 0 {
		sampleStats.PositionStats = make(map[int]*PositionDetail, refSeqLen)
		for pos := 1; pos <= refSeqLen; pos++ {
			sampleStats.PositionStats[pos] = &PositionDetail{}
		}
	}

	// localReadStats := NewSampleStats()

	// 预分配buffer减少GC
	var totalReads, alignedReads int
	alignedBasesThisRead := 0

	// 预分配临时映射和切片，减少每次循环中的内存分配
	var insertSubtypes = make(map[InsertionSubtype]bool)
	var deleteSubtypes = make(map[DeletionSubtype]bool)
	var substSubtypes = make(map[SubstitutionSubtype]bool)
	var deleteList []DeletionInfo

	// 遍历所有记录
	for {
		// 检查取消信号
		select {
		case <-ctx.Done():
			return ctx.Err() // 返回取消错误
		default:
		}

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

		// 清空临时映射和切片，重用内存
		for k := range insertSubtypes {
			delete(insertSubtypes, k)
		}
		for k := range deleteSubtypes {
			delete(deleteSubtypes, k)
		}
		for k := range substSubtypes {
			delete(substSubtypes, k)
		}
		deleteList = deleteList[:0]

		// 第一次遍历：收集位置统计和突变信息

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

					// 使用预定义的位置统计
					if posStats, ok := sampleStats.PositionStats[currentPos]; ok {
						posStats.Depth++

						if perfectUptoNow && !isMismatch && !hasInsertAfter {
							posStats.PerfectUptoPosCount++
						}

						if hasInsertAfter {
							if isMismatch {
								posStats.MismatchWithIns++
							} else {
								posStats.MatchWithIns++
							}
							posStats.Insertion++
						} else {
							if isMismatch {
								posStats.MismatchPure++
							} else {
								posStats.MatchPure++
							}
						}

						// N-mer统计
						if consecutiveMatch >= stats.SampleInfo.NMerSize {
							posStats.NCorrect++
							if !isMismatch {
								posStats.N1Correct++
							}
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
					if posStats, ok := sampleStats.PositionStats[pos]; ok {
						posStats.Depth++
						posStats.Deletion++
						if length == 1 {
							posStats.Del1++
						}
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
						if posStats, ok := sampleStats.PositionStats[pos]; ok {
							posStats.PerfectReadsCount++
						}
					}
					refPos += length
				case 2, 3:
					refPos += length
				}
			}
		}

		// 分析详细的read类型
		readInfo := analyzeReadDetailedInfo(read, mdMap, mdStr, sample.FullSeqs)
		sampleStats.ReadTypeCounts[readInfo.MainType]++

		mutationCount := len(readInfo.Mutations)
		sampleStats.SubstitutionCountDist[mutationCount]++
		sampleStats.AlignedBases += alignedBasesThisRead

		// 跳过非良好比对
		if mutationCount > stats.SampleInfo.MaxSubstitutions {
			continue
		}
		sampleStats.GoodAlignedReads++

		// 统计插入信息
		if readInfo.InsertSub != nil && len(readInfo.InsertSub.Insertions) > 0 {
			sampleStats.InsertReads++

			for _, insertion := range readInfo.InsertSub.Insertions {
				sampleStats.InsertEventCount++
				sampleStats.InsertBaseTotal += insertion.Length
				sampleStats.MutationBaseCounts["insertion"] += insertion.Length

				length := insertion.Length
				if length > 3 {
					length = 4
				}
				sampleStats.InsertLengthDist[length]++

				if insertion.Bases != "" {
					sampleStats.InsertBaseCounts[insertion.Bases]++
				}

				st := insertion.Subtype
				sampleStats.InsertSubtypeEvents[st]++
				sampleStats.InsertSubtypeBases[st] += insertion.Length
				insertSubtypes[st] = true
			}
			// 修复：InsertSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range insertSubtypes {
				sampleStats.InsertSubtypeReads[st]++
			}
		}

		// 统计突变信息
		if mutationCount > 0 {
			sampleStats.ReadsWithMutations++
			sampleStats.SubstitutionReads++
			sampleStats.SubstitutionEventCount += mutationCount
			sampleStats.SubstitutionBaseTotal += mutationCount
			sampleStats.MutationBaseCounts["substitution"] += mutationCount

			for _, mut := range readInfo.Mutations {
				posKey := strconv.Itoa(mut.Position)
				mutKey := mut.Ref + ">" + mut.Alt
				posMutKey := posKey + "_" + mutKey

				if _, ok := sampleStats.PositionDetails[posKey]; !ok {
					sampleStats.PositionDetails[posKey] = make(map[string]int)
				}
				sampleStats.PositionDetails[posKey][mutKey]++
				sampleStats.Mutations[mutKey]++
				sampleStats.PositionMutations[posMutKey]++
			}
		}

		// 缺失细分类统计
		if readInfo.DeleteSub != nil {
			if len(readInfo.DeleteSub.Deletions) > 0 {
				sampleStats.DeleteReads++
			}
			for _, del := range readInfo.DeleteSub.Deletions {
				sampleStats.DeleteEventCount++
				sampleStats.DeleteBaseTotal += del.Length
				sampleStats.MutationBaseCounts["deletion"] += del.Length

				length := del.Length
				if length > 3 {
					length = 4
				}
				sampleStats.DeleteLengthDist[length]++

				st := del.Subtype
				sampleStats.DeleteSubtypeEvents[st]++
				sampleStats.DeleteSubtypeBases[st] += del.Length
				deleteSubtypes[st] = true
				deleteList = append(deleteList, del)

				if st == Del1 && len(del.Bases) == 1 {
					base := del.Bases[0]
					sampleStats.Del1BaseCounts[base]++
					posKey := fmt.Sprintf("%d:%c", del.Position, base)
					sampleStats.DeletePositionCounts[posKey]++
				}

				// Del3 统计
				if st == Del3 && sample.FullSeqs != "" {
					pos := del.Position
					var prevBase byte
					var firstBase byte
					if pos-1 >= 1 && pos-1 <= len(sample.FullSeqs) {
						prevBase = sample.FullSeqs[pos-2]
						sampleStats.Del3PrevBaseCounts[prevBase]++
					}
					if len(del.Bases) > 0 {
						firstBase = del.Bases[0]
						sampleStats.Del3FirstBaseCounts[firstBase]++
					}
					combKey := fmt.Sprintf("%c>%c", prevBase, firstBase)
					sampleStats.Del3PrevFirstCombCounts[combKey]++
				}
			}
			// 修复：DeleteSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range deleteSubtypes {
				sampleStats.DeleteSubtypeReads[st]++
			}
		}

		// 替换细分类统计
		if len(readInfo.Mutations) > 0 {
			for _, sub := range readInfo.Mutations {
				st := sub.Subtype
				sampleStats.SubstitutionSubtypeEvents[st]++
				sampleStats.SubstitutionSubtypeBases[st]++
				substSubtypes[st] = true
			}
			// 修复：SubstitutionSubtypeReads 应该基于 Read 维度，每个 Read 只计数一次
			for st := range substSubtypes {
				sampleStats.SubstitutionSubtypeReads[st]++
			}
		}

		// 细分类组合统计
		if len(insertSubtypes) > 0 || len(deleteSubtypes) > 0 || len(substSubtypes) > 0 {
			combKey := buildSubtypeCombinationKey(insertSubtypes, deleteSubtypes, substSubtypes)
			sampleStats.SubtypeCombinationCounts[combKey]++
		}
	}

	// 更新样本统计 - 无需加锁，因为每个样本只对应一个BAM文件
	sampleStats.ReadCounts = totalReads
	sampleStats.AlignedReads = alignedReads
	sampleStats.RefLength = refSeqLen
	sampleStats.HeadCut = sampleHeadCut
	sampleStats.TailCut = sampleTailCut
	sampleStats.TotalMutations = sampleStats.SubstitutionEventCount // + sampleStats.InsertEventCount + sampleStats.DeleteEventCount
	// 平滑连续碱基缺失分配
	sampleStats.NormalizeDeletionsByContinuousBases()

	// 更新总统计
	stats.Lock()
	stats.TotalReadCount += totalReads
	stats.TotalAlignedReads += alignedReads
	stats.TotalReadsWithMuts += sampleStats.ReadsWithMutations

	// 合并 ACGT 统计
	if sample.FullSeqs != "" {
		stats.TotalRefACGTCounts['A'] += sampleStats.RefACGTCounts['A']
		stats.TotalRefACGTCounts['C'] += sampleStats.RefACGTCounts['C']
		stats.TotalRefACGTCounts['G'] += sampleStats.RefACGTCounts['G']
		stats.TotalRefACGTCounts['T'] += sampleStats.RefACGTCounts['T']
		stats.TotalRefLengthAfterTrim += sampleStats.RefLengthAfterTrim
	}

	// 合并样本统计到总统计 - 使用批量操作

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

	stats.Unlock()

	return nil
}

func (stats *MutationStats) ProcessBAMFiles() (err error) {
	sampleInfo := stats.SampleInfo

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 确保所有资源释放

	g, ctx := errgroup.WithContext(ctx)

	// 信号量 channel，容量即为最大并发数
	sem := make(chan struct{}, sampleInfo.MaxThreads)

	// 处理每个BAM文件
	for _, sampleName := range sampleInfo.Order {
		sample, ok := sampleInfo.Samples[sampleName]
		if !ok {
			slog.Error("sampleInfo的Order包含Samples中未定义的样本", "样品", sampleName)
			err = fmt.Errorf("样本 %s 未在sampleInfo.Samples中定义", sampleName)
			return
		}

		// 获取信号量，阻塞直到有空闲槽位
		sem <- struct{}{}
		g.Go(func() error {
			// 任务结束后释放信号量
			defer func() { <-sem }()

			return stats.ProcessBAMFile(ctx, sample)
		})
	}

	// 等待所有 goroutine 完成，返回第一个错误
	return g.Wait()
}
