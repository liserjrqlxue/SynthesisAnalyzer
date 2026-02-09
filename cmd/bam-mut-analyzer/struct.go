package main

import "sync"

// MutationStats 存储突变统计
type MutationStats struct {
	sync.RWMutex
	PositionMutations    map[string]map[string]int            // sample -> position:mutation -> count
	SampleMutations      map[string]map[string]int            // sample -> mutation -> count
	TotalMutations       map[string]int                       // mutation -> total count
	PositionDetails      map[string]map[string]map[string]int // sample -> position -> mutation -> count
	SampleReadCounts     map[string]int                       // sample -> total reads count
	SampleReadsWithMuts  map[string]int                       // sample -> 包含突变的reads数
	SampleReadTypeCounts map[string]map[ReadType]int          // sample -> read type -> count
	TotalReadCount       int                                  // 所有样本的总reads数
	TotalReadsWithMuts   int                                  // 所有样本的包含突变reads数的和
	TotalReadTypeCounts  map[ReadType]int                     // 所有样本的read type统计
}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	return &MutationStats{
		PositionMutations:    make(map[string]map[string]int),
		SampleMutations:      make(map[string]map[string]int),
		TotalMutations:       make(map[string]int),
		PositionDetails:      make(map[string]map[string]map[string]int),
		SampleReadCounts:     make(map[string]int),
		SampleReadsWithMuts:  make(map[string]int),
		SampleReadTypeCounts: make(map[string]map[ReadType]int),
		TotalReadCount:       0,
		TotalReadsWithMuts:   0,
		TotalReadTypeCounts:  make(map[ReadType]int),
	}
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
