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

// ReadDetailedType 详细的read类型信息
type ReadDetailedType struct {
	MainType    ReadType
	InsertSub   *InsertSubtype
	DeleteSub   *DeleteSubtype
	HasMutation bool
}

// MutationStats 存储突变统计 - 增加缺失碱基统计
type MutationStats struct {
	sync.RWMutex
	PositionMutations        map[string]map[string]int            // sample -> position:mutation -> count
	SampleMutations          map[string]map[string]int            // sample -> mutation -> count
	TotalMutations           map[string]int                       // mutation -> total count
	PositionDetails          map[string]map[string]map[string]int // sample -> position -> mutation -> count
	SampleReadCounts         map[string]int                       // sample -> total reads count
	SampleAlignedReads       map[string]int                       // sample -> 比对上的reads数
	SampleReadsWithMuts      map[string]int                       // sample -> 包含突变的reads数
	SampleReadTypeCounts     map[string]map[ReadType]int          // sample -> read type -> count
	SampleDetailedTypeCounts map[string]map[string]int            // sample -> 详细类型 -> count
	SampleInsertLengthDist   map[string]map[int]int               // sample -> 插入长度 -> count
	SampleDeleteLengthDist   map[string]map[int]int               // sample -> 缺失长度 -> count
	TotalReadCount           int                                  // 所有样本的总reads数
	TotalAlignedReads        int                                  // 所有样本的比对reads数
	TotalReadsWithMuts       int                                  // 所有样本的包含突变reads数的和
	TotalReadTypeCounts      map[ReadType]int                     // 所有样本的read type统计
	TotalDetailedTypeCounts  map[string]int                       // 所有样本的详细类型统计
	TotalInsertLengthDist    map[int]int                          // 所有样本的插入长度分布
	TotalDeleteLengthDist    map[int]int                          // 所有样本的缺失长度分布
	TotalMaxDeleteLengthDist map[int]int                          // 所有样本的最长缺失长度分布
	MutationBaseCounts       map[string]map[string]int            // 碱基维度统计: mutation_type -> count

	SampleDeleteBaseCounts map[string]map[byte]int   // sample -> 缺失碱基 -> count（长度为1的缺失）
	TotalDeleteBaseCounts  map[byte]int              // 所有样本的缺失碱基统计
	SampleInsertBaseCounts map[string]map[string]int // sample -> 插入序列 -> count
	TotalInsertBaseCounts  map[string]int            // 所有样本的插入序列统计

	SampleDeletePositionCounts map[string]map[string]int // sample -> "位置:碱基" -> count
	TotalDeletePositionCounts  map[string]int            // 所有样本的缺失位置统计
}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	stats := &MutationStats{
		PositionMutations:        make(map[string]map[string]int),
		SampleMutations:          make(map[string]map[string]int),
		TotalMutations:           make(map[string]int),
		PositionDetails:          make(map[string]map[string]map[string]int),
		SampleReadCounts:         make(map[string]int),
		SampleAlignedReads:       make(map[string]int),
		SampleReadsWithMuts:      make(map[string]int),
		SampleReadTypeCounts:     make(map[string]map[ReadType]int),
		SampleDetailedTypeCounts: make(map[string]map[string]int),
		SampleInsertLengthDist:   make(map[string]map[int]int),
		SampleDeleteLengthDist:   make(map[string]map[int]int),
		TotalReadCount:           0,
		TotalAlignedReads:        0,
		TotalReadsWithMuts:       0,
		TotalReadTypeCounts:      make(map[ReadType]int),
		TotalDetailedTypeCounts:  make(map[string]int),
		TotalInsertLengthDist:    make(map[int]int),
		TotalDeleteLengthDist:    make(map[int]int),
		TotalMaxDeleteLengthDist: make(map[int]int),
		MutationBaseCounts:       make(map[string]map[string]int),

		SampleDeleteBaseCounts: make(map[string]map[byte]int),
		TotalDeleteBaseCounts:  make(map[byte]int),
		SampleInsertBaseCounts: make(map[string]map[string]int),
		TotalInsertBaseCounts:  make(map[string]int),

		SampleDeletePositionCounts: make(map[string]map[string]int),
		TotalDeletePositionCounts:  make(map[string]int),
	}
	return stats
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
