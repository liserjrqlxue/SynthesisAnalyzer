package stats

import "sync"

// SampleStats 单个样本的统计信息 - 添加新字段
type SampleStats struct {
	sync.RWMutex
	PositionMutations map[string]int            // position:mutation -> count
	Mutations         map[string]int            // mutation -> count
	PositionDetails   map[string]map[string]int // position -> mutation -> count

	ReadTypeCounts       map[ReadType]int // read type -> count
	InsertLengthDist     map[int]int      // 插入长度 -> count
	DeleteLengthDist     map[int]int      // 缺失长度 -> count
	InsertBaseCounts     map[string]int   // 插入序列 -> count
	DeletePositionCounts map[string]int   // "位置:碱基" -> count
	MutationBaseCounts   map[string]int   // 碱基维度变异统计
	MutationList         []Mutation       // 所有突变列表（可选，用于调试）

	ReadCounts         int // 总reads数
	AlignedReads       int // 比对上的reads数
	ReadsWithMutations int // 包含突变的reads数

	// 新增：reads维度的变异统计
	InsertReads       int // 包含插入的reads数
	DeleteReads       int // 包含缺失的reads数
	SubstitutionReads int // 包含替换的reads数

	TotalMutations int //总突变数
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

	RefSeqFull         string       // 完整的原始参考序列
	RefLength          int          // 参考序列全长
	HeadCut            int          // 头切除长度（默认或Excel指定）
	TailCut            int          // 尾切除长度（默认或Excel指定）
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
		Mutations:            make(map[string]int),
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
	if s.RefSeqFull == "" || s.RefLength == 0 {
		return // 无参考序列信息，无法修正
	}
	if s.HeadCut < 0 || s.TailCut < 0 || s.HeadCut+s.TailCut >= s.RefLength {
		return // 切除参数不合理
	}
	trimmedRef := s.RefSeqFull[s.HeadCut : s.RefLength-s.TailCut]
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
			rawPos := pTrim + s.HeadCut
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
			rawPos := pTrim + s.HeadCut
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
