package main

import "sync"

// ========== 新增：位置详细统计结构 ==========
type PositionDetail struct {
	Depth               int
	MatchPure           int
	MatchWithIns        int
	MismatchPure        int
	MismatchWithIns     int
	Insertion           int
	Deletion            int
	Del1                int
	PerfectReadsCount   int
	PerfectUptoPosCount int
	AlignedSum          int // 覆盖该位置的所有样品的 aligned reads 之和
	TotalSum            int // 覆盖该位置的所有样品的 total reads 之和
}

// InsertSubtype 插入子类型（多条插入）
type InsertSubtype struct {
	Insertions []InsertionInfo // 每条插入的信息
}

// InsertionInfo 单条插入信息
type InsertionInfo struct {
	Length           int              // 插入长度
	IsSameAsFlanking bool             // 插入字符是否与插入位置两边碱基相同
	Bases            string           // 插入的碱基序列
	Subtype          InsertionSubtype // 细分类
}

// DeleteSubtype 缺失子类型
type DeleteSubtype struct {
	Deletions []DeletionInfo // 每条缺失的信息
}

// DeletionInfo 单条缺失信息
type DeletionInfo struct {
	Length   int             // 缺失长度
	Bases    string          // 缺失的碱基序列（如果长度为1）
	Position int             // 缺失位置（1-based）
	Subtype  DeletionSubtype // 细分类
}

// ReadDetailedInfo 详细的read信息
type ReadDetailedInfo struct {
	MainType       ReadType
	InsertSub      *InsertSubtype
	DeleteSub      *DeleteSubtype
	Mutations      []Mutation         // 该read中的所有突变
	Substitutions  []SubstitutionInfo // 替换细分类列表
	SubtypeTags    []string           // 该read的所有细分类标签（去重后排序用）
	CombinationKey string             // 细分类组合唯一键（排序后拼接）
}

type PositionNMerStats struct {
	NCorrect  int
	N1Correct int
}

// SampleStats 单个样本的统计信息 - 添加新字段
type SampleStats struct {
	sync.RWMutex
	PositionMutations map[string]int            // position:mutation -> count
	Mutations         map[string]int            // mutation -> count
	PositionDetails   map[string]map[string]int // position -> mutation -> count

	ReadTypeCounts       map[ReadType]int // read type -> count
	DetailedTypeCounts   map[string]int   // 详细类型 -> count
	InsertLengthDist     map[int]int      // 插入长度 -> count
	DeleteLengthDist     map[int]int      // 缺失长度 -> count
	MaxDeleteLengthDist  map[int]int      // 最长缺失长度 -> count
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

	PositionStats map[int]*PositionDetail

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

	NMerStats map[int]*PositionNMerStats // 1-base

	AlignedBases int // 样本所有比对read的总参考覆盖碱基数
}

// NewSampleStats 创建新的样本统计对象
func NewSampleStats() *SampleStats {
	return &SampleStats{
		PositionMutations:    make(map[string]int),
		Mutations:            make(map[string]int),
		PositionDetails:      make(map[string]map[string]int),
		ReadTypeCounts:       make(map[ReadType]int),
		DetailedTypeCounts:   make(map[string]int),
		InsertLengthDist:     make(map[int]int),
		DeleteLengthDist:     make(map[int]int),
		MaxDeleteLengthDist:  make(map[int]int),
		InsertBaseCounts:     make(map[string]int),
		DeletePositionCounts: make(map[string]int),
		MutationBaseCounts:   make(map[string]int),

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

		PositionStats:           make(map[int]*PositionDetail),
		SubstitutionCountDist:   make(map[int]int),
		Del1BaseCounts:          make(map[byte]int),
		RefACGTCounts:           make(map[byte]int),
		Del3PrevBaseCounts:      make(map[byte]int),
		Del3FirstBaseCounts:     make(map[byte]int),
		Del3PrevFirstCombCounts: make(map[string]int),

		NMerStats: make(map[int]*PositionNMerStats),
	}
}

// MutationStats 存储突变统计 - 添加新字段
type MutationStats struct {
	sync.RWMutex
	Samples map[string]*SampleStats // sample -> 样本统计

	TotalMutations          map[string]int   // mutation -> total count
	TotalReadTypeCounts     map[ReadType]int // 所有样本的read type统计
	TotalDetailedTypeCounts map[string]int   // 所有样本的详细类型统计

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
		TotalDetailedTypeCounts:  make(map[string]int),
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

// getOrCreateSampleStats 获取或创建样本统计对象
func (stats *MutationStats) getOrCreateSampleStats(sampleName string) *SampleStats {
	stats.Lock()
	defer stats.Unlock()

	if sampleStats, ok := stats.Samples[sampleName]; ok {
		return sampleStats
	}

	sampleStats := NewSampleStats()
	stats.Samples[sampleName] = sampleStats
	return sampleStats
}

// Mutation 表示一个碱基突变
type Mutation struct {
	Position int
	Ref      string
	Alt      string
}

// 新增：替换信息结构（用于记录替换细分类）
type SubstitutionInfo struct {
	Position int
	Ref      string
	Alt      string
	Subtype  SubstitutionSubtype
}

// MDOp 表示MD标签的操作
type MDOp struct {
	Type   byte
	Length int
	Base   byte
	Bases  string
}
