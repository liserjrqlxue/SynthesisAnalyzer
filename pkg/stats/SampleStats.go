package stats

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
)

// SampleStats 单个样本的统计信息 - 添加新字段
type SampleStats struct {
	sync.RWMutex
	Sample *Sample // 样本信息

	PositionMutations map[string]int            // position:mutation -> count
	PositionDetails   map[string]map[string]int // position -> mutation -> count

	ReadTypeCounts    map[ReadType]int // read type -> count
	SubstitutionCount map[string]int   // mutation -> count

	InsertLengthDist map[int]int    // 插入长度 -> count
	InsertBaseCounts map[string]int // 插入序列 -> count

	DeleteLengthDist     map[int]int    // 缺失长度 -> count
	DeletePositionCounts map[string]int // "位置:碱基" -> count
	MutationBaseCounts   map[string]int // 碱基维度变异统计
	MutationList         []Mutation     // 所有突变列表（可选，用于调试）

	ReadCounts         int // 总reads数
	AlignedReads       int // 比对上的reads数
	ReadsWithMutations int // 包含突变的reads数

	// 新增：reads维度的变异统计
	InsertReads       int // 包含插入的reads数
	DeleteReads       int // 包含缺失的reads数
	SubstitutionReads int // 包含替换的reads数

	TotalSubstitution int //总突变数
	// 新增：变异事件个数维度
	InsertEventCount       int // 插入事件个数（多条插入算多个）
	DeleteEventCount       int // 缺失事件个数（多条缺失算多个）
	SubstitutionEventCount int // 替换事件个数（多条替换算多个）

	// 新增：碱基个数维度
	InsertBaseTotal       int // 插入碱基总数（所有插入长度的累加）
	DeleteBaseTotal       int // 缺失碱基总数（所有缺失长度的累加）
	SubstitutionBaseTotal int // 替换碱基总数（替换个数）

	// 新增：缺失细分类统计（三个维度）
	DeleteSubtypeReads  map[DeletionSubtype]int // 包含该细分类缺失的reads数
	DeleteSubtypeEvents map[DeletionSubtype]int // 该细分类缺失事件总数
	DeleteSubtypeBases  map[DeletionSubtype]int // 该细分类缺失碱基总数
	// 新增：Del1缺失碱基分布
	Del1BaseCounts map[byte]int // 缺失碱基 -> 事件数（注意：事件数即缺失个数）

	// 新增：插入细分类统计
	InsertSubtypeReads  map[InsertionSubtype]int
	InsertSubtypeEvents map[InsertionSubtype]int
	InsertSubtypeBases  map[InsertionSubtype]int

	// 新增：替换细分类统计
	SubstitutionSubtypeReads  map[SubstitutionSubtype]int
	SubstitutionSubtypeEvents map[SubstitutionSubtype]int
	SubstitutionSubtypeBases  map[SubstitutionSubtype]int // 替换碱基数，总是1

	// 新增：细分类组合统计（reads维度）
	SubtypeCombinationCounts map[string]int // 组合键 -> reads数

	// RefSeqFull         string       // 完整的原始参考序列 -> 从Sample中获取
	// RefLength          int          // 参考序列全长 -> 从Sample中获取
	// HeadCut            int          // 头切除长度（默认或Excel指定） -> 从Sample中获取
	// TailCut            int          // 尾切除长度（默认或Excel指定） -> 从Sample中获取
	RefACGTCounts      map[byte]int // 切除头尾后参考序列中A、C、G、T的计数
	RefLengthAfterTrim int          // 切除头尾后的参考长度

	SubstitutionCountDist map[int]int // 替换个数 -> 该个数的reads数
	GoodAlignedReads      int         // 比对良好reads数（替换个数 <= 阈值）

	// 新增：Del3 缺失前一个碱基分布
	Del3PrevBaseCounts map[byte]int
	// 新增：Del3 缺失第一个碱基分布
	Del3FirstBaseCounts     map[byte]int
	Del3PrevFirstCombCounts map[string]int // 组合计数，键如 "AC"

	// 新增：位置统计信息 1-based
	PositionStats map[int]*PositionDetail

	AlignedBases int // 样本所有比对read的总参考覆盖碱基数
}

// NewSampleStats 创建新的样本统计对象
func NewSampleStats() *SampleStats {
	return &SampleStats{
		PositionMutations:    make(map[string]int),
		SubstitutionCount:    make(map[string]int),
		PositionDetails:      make(map[string]map[string]int),
		ReadTypeCounts:       make(map[ReadType]int),
		InsertLengthDist:     make(map[int]int),
		DeleteLengthDist:     make(map[int]int),
		InsertBaseCounts:     make(map[string]int),
		DeletePositionCounts: make(map[string]int),
		MutationBaseCounts:   make(map[string]int),

		// 细分类统计初始化
		DeleteSubtypeReads:        make(map[DeletionSubtype]int),
		DeleteSubtypeEvents:       make(map[DeletionSubtype]int),
		DeleteSubtypeBases:        make(map[DeletionSubtype]int),
		InsertSubtypeReads:        make(map[InsertionSubtype]int),
		InsertSubtypeEvents:       make(map[InsertionSubtype]int),
		InsertSubtypeBases:        make(map[InsertionSubtype]int),
		SubstitutionSubtypeReads:  make(map[SubstitutionSubtype]int),
		SubstitutionSubtypeEvents: make(map[SubstitutionSubtype]int),
		SubstitutionSubtypeBases:  make(map[SubstitutionSubtype]int),
		SubtypeCombinationCounts:  make(map[string]int),

		SubstitutionCountDist:   make(map[int]int),
		Del1BaseCounts:          make(map[byte]int),
		RefACGTCounts:           make(map[byte]int),
		Del3PrevBaseCounts:      make(map[byte]int),
		Del3FirstBaseCounts:     make(map[byte]int),
		Del3PrevFirstCombCounts: make(map[string]int),

		PositionStats: make(map[int]*PositionDetail),
		// MutationList: make([]Mutation, 0, 64),
	}
}

// NormalizeDeletionsByContinuousBases 对切除头尾后参考序列中的连续相同碱基区间，
// 将 PositionStats 中对应位置的 Deletion 值修正为：区间内平均缺失率 * 该位置 Depth。
// 缺失率 = Deletion / Depth，平均缺失率取区间内各位置缺失率的算术平均。
func (s *SampleStats) NormalizeDeletionsByContinuousBases() {

	// 获取切除后的参考序列
	if s.Sample.RefSeqFull == "" || s.Sample.RefLength == 0 {
		return // 无参考序列信息，无法修正
	}
	if s.Sample.HeadCut < 0 || s.Sample.TailCut < 0 || s.Sample.HeadCut+s.Sample.TailCut >= s.Sample.RefLength {
		return // 切除参数不合理
	}
	trimmedRef := s.Sample.RefSeqFull[s.Sample.HeadCut : s.Sample.RefLength-s.Sample.TailCut]
	refLen := len(trimmedRef)

	// 遍历参考序列的每个位置（1-based 对应切除后的坐标，但 PositionStats 使用原始坐标）
	// 我们需要按原始坐标操作，因此映射关系：原始坐标 = trimmedPos + HeadCut
	pos := 1 // 切除后序列中的1-based位置
	for pos <= refLen {
		// 确定当前连续碱基区间在切除后序列中的起始和结束（1-based）
		startTrim := pos
		currentBase := trimmedRef[startTrim-1]
		endTrim := startTrim
		for endTrim < refLen && trimmedRef[endTrim] == currentBase {
			endTrim++
		}
		// 区间长度
		// intervalLen := endTrim - startTrim + 1

		// 收集区间内所有原始位置的 Depth 和 Deletion
		var depths []int
		var deletions []int
		var del1s []int
		validPositions := 0
		for pTrim := startTrim; pTrim <= endTrim; pTrim++ {
			rawPos := pTrim + s.Sample.HeadCut
			if detail, ok := s.PositionStats[rawPos]; ok && detail.Depth > 0 {
				depths = append(depths, detail.Depth)
				deletions = append(deletions, detail.Deletion)
				del1s = append(del1s, detail.Del1)
				validPositions++
			}
		}
		if validPositions == 0 {
			pos = endTrim + 1
			continue
		}

		// 计算平均缺失率
		var sumRate float64
		var sumRate1 float64
		for i := range validPositions {
			rate1 := float64(deletions[i]) / float64(depths[i])
			sumRate += rate1
			sumRate1 += float64(del1s[i]) / float64(depths[i])
		}
		avgRate := sumRate / float64(validPositions)
		avgRate1 := sumRate1 / float64(validPositions)

		// 将区间内每个位置的 Deletion 更新为 avgRate * Depth
		for pTrim := startTrim; pTrim <= endTrim; pTrim++ {
			rawPos := pTrim + s.Sample.HeadCut
			detail, ok := s.PositionStats[rawPos]
			if !ok {
				// 如果该位置原本不存在（深度为0），则忽略
				continue
			}
			detail.Base = currentBase
			if detail.Depth > 0 {
				// 新 Deletion = avgRate * Depth，四舍五入
				newDel := int(avgRate*float64(detail.Depth) + 0.5)
				detail.Deletion = newDel
				newDel1 := int(avgRate1*float64(detail.Depth) + 0.5)
				detail.Del1 = newDel1
				/* 		} else {
				// 深度为0时，Deletion 应为0（但可以保持原值0）
				detail.Deletion = 0 */
			}
		}

		// 移动到下一区间
		pos = endTrim + 1
	}
}

// ProcessBAMFile 处理单个BAM文件 - 优化版本（支持取消）
func (sampleStats *SampleStats) ProcessBAMFile(ctx context.Context, NMerSize, MaxSubstitutions int) error {
	sample := sampleStats.Sample
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

	// 检查参考序列长度是否与BAM头中一致
	// 一致则继续，不一致则报错
	refSeqLen := 0
	for _, ref := range br.Header().Refs() {
		if ref.Name() == sample.Name {
			refSeqLen = ref.Len()
			break
		}
	}
	if refSeqLen == 0 {
		slog.Warn("无法获取样本参考序列长度", "样品", sample.Name)
	} else if refSeqLen != sampleStats.Sample.RefLength {
		slog.Error("参考序列长度冲突", "样品", sample.Name, "refSeqLen", refSeqLen, "sampleStats.RefLength", sampleStats.Sample.RefLength)
		return fmt.Errorf("参考序列长度冲突: %d vs. %d", refSeqLen, sampleStats.Sample.RefLength)
	}

	err = sampleStats.ProcessBAMReader(ctx, br, NMerSize, MaxSubstitutions)
	if err != nil {
		return err
	}

	// 更新样本统计 - 无需加锁，因为每个样本只对应一个BAM文件
	sampleStats.TotalSubstitution = sampleStats.SubstitutionEventCount // + sampleStats.InsertEventCount + sampleStats.DeleteEventCount
	// 平滑连续碱基缺失分配
	sampleStats.NormalizeDeletionsByContinuousBases()

	return nil
}

// ProcessBAMReader 处理BAM读取器中的记录
func (sampleStats *SampleStats) ProcessBAMReader(ctx context.Context, br *bam.Reader, NMerSize, MaxSubstitutions int) error {
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
		err = sampleStats.ProcessSamRecord(read, NMerSize, MaxSubstitutions)
		if err != nil {
			return err
		}
	}
	return nil
}

// ProcessSamRecord 处理单挑SAM记录
func (sampleStats *SampleStats) ProcessSamRecord(read *sam.Record, NMerSize, MaxSubstitutions int) error {
	sampleStats.ReadCounts++

	var (
		insertSubtypes = make(map[InsertionSubtype]bool)
		deleteSubtypes = make(map[DeletionSubtype]bool)
		substSubtypes  = make(map[SubstitutionSubtype]bool)

		alignedBasesThisRead = 0
	)

	// 跳过未比对记录
	if read.Flags&sam.Unmapped != 0 {
		return nil
	}
	sampleStats.AlignedReads++

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
					if consecutiveMatch >= NMerSize {
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
	readInfo := analyzeReadDetailedInfo(read, mdMap, mdStr, sampleStats.Sample.RefSeqFull)
	sampleStats.ReadTypeCounts[readInfo.MainType]++

	mutationCount := len(readInfo.Mutations)
	sampleStats.SubstitutionCountDist[mutationCount]++
	sampleStats.AlignedBases += alignedBasesThisRead

	// 跳过非良好比对
	if mutationCount > MaxSubstitutions {
		return nil
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
			sampleStats.SubstitutionCount[mutKey]++
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

			if st == Del1 && len(del.Bases) == 1 {
				base := del.Bases[0]
				sampleStats.Del1BaseCounts[base]++
				posKey := fmt.Sprintf("%d:%c", del.Position, base)
				sampleStats.DeletePositionCounts[posKey]++
			}

			// Del3 统计
			if st == Del3 && sampleStats.Sample.RefSeqFull != "" {
				pos := del.Position
				var prevBase byte
				var firstBase byte
				if pos-1 >= 1 && pos-1 <= len(sampleStats.Sample.RefSeqFull) {
					prevBase = sampleStats.Sample.RefSeqFull[pos-2]
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
	return nil
}
