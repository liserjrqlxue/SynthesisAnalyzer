package stats

import (
	"sort"
	"sync"
)

// MutationStats 存储突变统计 - 添加新字段
type MutationStats struct {
	sync.RWMutex
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
func (stats *MutationStats) SortSampleNames(sampleOrder []string) {
	var names = stats.SampleNames
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
