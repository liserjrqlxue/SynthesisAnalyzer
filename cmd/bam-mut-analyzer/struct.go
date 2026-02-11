package main

import "sync"

// InsertSubtype 插入子类型（多条插入）
type InsertSubtype struct {
	Insertions []InsertionInfo // 每条插入的信息
}

// InsertionInfo 单条插入信息
type InsertionInfo struct {
	Length           int    // 插入长度
	IsSameAsFlanking bool   // 插入字符是否与插入位置两边碱基相同
	Bases            string // 插入的碱基序列
}

// DeleteSubtype 缺失子类型
type DeleteSubtype struct {
	Deletions []DeletionInfo // 每条缺失的信息
}

// DeletionInfo 单条缺失信息
type DeletionInfo struct {
	Length   int    // 缺失长度
	Bases    string // 缺失的碱基序列（如果长度为1）
	Position int    // 缺失位置（1-based）
}

// ReadDetailedInfo 详细的read信息
type ReadDetailedInfo struct {
	MainType  ReadType
	InsertSub *InsertSubtype
	DeleteSub *DeleteSubtype
	Mutations []Mutation // 该read中的所有突变
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
	DeleteBaseCounts     map[byte]int     // 缺失碱基 -> count（长度为1的缺失）
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
		DeleteBaseCounts:     make(map[byte]int),
		InsertBaseCounts:     make(map[string]int),
		DeletePositionCounts: make(map[string]int),
		MutationBaseCounts:   make(map[string]int),
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

// MDOp 表示MD标签的操作
type MDOp struct {
	Type   byte
	Length int
	Base   byte
	Bases  string
}
