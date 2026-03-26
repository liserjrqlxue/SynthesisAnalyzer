package stats

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

// MutationStats 存储突变统计 - 添加新字段
type MutationStats struct {
	sync.RWMutex
	BatchInfo   *BatchInfo              // 样本信息指针
	Samples     map[string]*SampleStats // sample -> 样本统计
	SampleNames []string                // 样本名顺序

	ReadTypeCounts map[ReadType]int // 所有样本的read type统计

	InsertLengthDist map[int]int    // 所有样本的插入长度分布
	InsertBaseCounts map[string]int // 所有样本的插入序列统计

	DeleteLengthDist     map[int]int    // 所有样本的缺失长度分布
	DeletePositionCounts map[string]int // 所有样本的缺失位置统计
	Del1BaseCounts       map[byte]int   // 总Del1缺失碱基分布

	SubstitutionCount map[string]int // mutation -> total count

	// 细分类统计
	DeleteSubtypeReads        map[DeletionSubtype]int
	DeleteSubtypeEvents       map[DeletionSubtype]int
	DeleteSubtypeBases        map[DeletionSubtype]int
	InsertSubtypeReads        map[InsertionSubtype]int
	InsertSubtypeEvents       map[InsertionSubtype]int
	InsertSubtypeBases        map[InsertionSubtype]int
	SubstitutionSubtypeReads  map[SubstitutionSubtype]int
	SubstitutionSubtypeEvents map[SubstitutionSubtype]int
	SubstitutionSubtypeBases  map[SubstitutionSubtype]int
	SubtypeCombinationCounts  map[string]int // 总体组合统计

	SubstitutionCountDist map[int]int // 替换个数分布

	Del3PrevBaseCounts      map[byte]int   // 总 Del3 缺失前一个碱基分布
	Del3FirstBaseCounts     map[byte]int   // 总 Del3 缺失第一个碱基分布
	Del3PrevFirstCombCounts map[string]int // 总 Del3 缺失前一个碱基组合统计

	TotalMaxDeleteLengthDist map[int]int  // 所有样本的最长缺失长度分布
	TotalDeleteBaseCounts    map[byte]int // 所有样本的缺失碱基统计

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

	TotalGoodAlignedReads int // 总体比对良好reads数
	// 新增：RefLength * GoodAlignedReads 的累加（用于比例计算）
	TotalRefLengthGoodAligned int // 所有样本的 RefLength * GoodAlignedReads 之和
	TotalAlignedBases         int // 所有样本的 AlignedBases 之和

	TotalRefACGTCounts      map[byte]int // 所有样本切除后参考中ACGT计数之和
	TotalRefLengthAfterTrim int          // 所有样本切除后参考长度之和

}

// NewMutationStats 创建新的统计对象
func NewMutationStats() *MutationStats {
	stats := &MutationStats{
		Samples:                  make(map[string]*SampleStats),
		SubstitutionCount:        make(map[string]int),
		ReadTypeCounts:           make(map[ReadType]int),
		InsertLengthDist:         make(map[int]int),
		DeleteLengthDist:         make(map[int]int),
		TotalMaxDeleteLengthDist: make(map[int]int),

		TotalDeleteBaseCounts: make(map[byte]int),
		InsertBaseCounts:      make(map[string]int),

		DeletePositionCounts: make(map[string]int),

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
		TotalRefACGTCounts:      make(map[byte]int),
		Del3PrevBaseCounts:      make(map[byte]int),
		Del3FirstBaseCounts:     make(map[byte]int),
		Del3PrevFirstCombCounts: make(map[string]int),
	}
	return stats
}

// GetOrCreateSampleStats 获取或创建样本统计对象
func (stats *MutationStats) GetOrCreateSampleStats(sample *Sample) *SampleStats {
	stats.Lock()
	defer stats.Unlock()

	if sampleStats, ok := stats.Samples[sample.Name]; ok {
		return sampleStats
	}

	// 初始化样本统计对象
	sampleStats := sample.NewSampleStats()

	stats.Samples[sample.Name] = sampleStats
	stats.SampleNames = append(stats.SampleNames, sample.Name)
	return sampleStats
}

// 辅助函数：获取排序后的样本名列表
func (stats *MutationStats) SortSampleNames() {
	var (
		names       = stats.SampleNames
		sampleOrder = stats.BatchInfo.Order
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

func (stats *MutationStats) ProcessBAMFiles() (err error) {
	sampleInfo := stats.BatchInfo

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

			// 初始化样本统计
			sampleStats := stats.GetOrCreateSampleStats(sample)
			err := sampleStats.ProcessBAMFile(ctx, stats.BatchInfo.NMerSize, stats.BatchInfo.MaxSubstitutions)
			if err != nil {
				return err
			}
			stats.UpdateStatsFromSampleStats(sampleStats)
			return nil
		})
	}

	// 等待所有 goroutine 完成，返回第一个错误
	return g.Wait()
}

func (stats *MutationStats) UpdateStatsFromSampleStats(sampleStats *SampleStats) {
	stats.Lock()
	defer stats.Unlock()

	stats.TotalReadCount += sampleStats.ReadCounts
	stats.TotalAlignedReads += sampleStats.AlignedReads
	stats.TotalReadsWithMuts += sampleStats.ReadsWithMutations

	// 合并 ACGT 统计
	if sampleStats.Sample.RefSeqFull != "" {
		stats.TotalRefACGTCounts['A'] += sampleStats.RefACGTCounts['A']
		stats.TotalRefACGTCounts['C'] += sampleStats.RefACGTCounts['C']
		stats.TotalRefACGTCounts['G'] += sampleStats.RefACGTCounts['G']
		stats.TotalRefACGTCounts['T'] += sampleStats.RefACGTCounts['T']
		stats.TotalRefLengthAfterTrim += sampleStats.RefLengthAfterTrim
	}

	// 合并read类型统计
	MergeMaps(stats.ReadTypeCounts, sampleStats.ReadTypeCounts)

	// 合并插入统计
	stats.TotalInsertReads += sampleStats.InsertReads
	stats.TotalInsertEventCount += sampleStats.InsertEventCount
	stats.TotalInsertBaseTotal += sampleStats.InsertBaseTotal
	MergeMaps(stats.InsertLengthDist, sampleStats.InsertLengthDist)
	MergeMaps(stats.InsertBaseCounts, sampleStats.InsertBaseCounts)

	// 合并缺失统计
	stats.TotalDeleteReads += sampleStats.DeleteReads
	stats.TotalDeleteEventCount += sampleStats.DeleteEventCount
	stats.TotalDeleteBaseTotal += sampleStats.DeleteBaseTotal
	MergeMaps(stats.DeleteLengthDist, sampleStats.DeleteLengthDist)
	MergeMaps(stats.DeletePositionCounts, sampleStats.DeletePositionCounts)
	MergeMaps(stats.Del1BaseCounts, sampleStats.Del1BaseCounts)

	// 合并替换统计
	stats.TotalSubstitutionReads += sampleStats.SubstitutionReads
	stats.TotalSubstitutionEventCount += sampleStats.SubstitutionEventCount
	stats.TotalSubstitutionBaseTotal += sampleStats.SubstitutionBaseTotal
	MergeMaps(stats.SubstitutionCount, sampleStats.SubstitutionCount)

	// 合并细分类统计
	MergeMaps(stats.DeleteSubtypeReads, sampleStats.DeleteSubtypeReads)
	MergeMaps(stats.DeleteSubtypeEvents, sampleStats.DeleteSubtypeEvents)
	MergeMaps(stats.DeleteSubtypeBases, sampleStats.DeleteSubtypeBases)
	MergeMaps(stats.InsertSubtypeReads, sampleStats.InsertSubtypeReads)
	MergeMaps(stats.InsertSubtypeEvents, sampleStats.InsertSubtypeEvents)
	MergeMaps(stats.InsertSubtypeBases, sampleStats.InsertSubtypeBases)
	MergeMaps(stats.SubstitutionSubtypeReads, sampleStats.SubstitutionSubtypeReads)
	MergeMaps(stats.SubstitutionSubtypeEvents, sampleStats.SubstitutionSubtypeEvents)
	MergeMaps(stats.SubstitutionSubtypeBases, sampleStats.SubstitutionSubtypeBases)
	MergeMaps(stats.SubtypeCombinationCounts, sampleStats.SubtypeCombinationCounts)

	// 合并替换个数分布
	MergeMaps(stats.SubstitutionCountDist, sampleStats.SubstitutionCountDist)

	// 合并Del3统计
	MergeMaps(stats.Del3PrevBaseCounts, sampleStats.Del3PrevBaseCounts)
	MergeMaps(stats.Del3FirstBaseCounts, sampleStats.Del3FirstBaseCounts)
	MergeMaps(stats.Del3PrevFirstCombCounts, sampleStats.Del3PrevFirstCombCounts)

	// 合并其他统计
	stats.TotalGoodAlignedReads += sampleStats.GoodAlignedReads
	trimmedLen := sampleStats.Sample.RefLength - sampleStats.Sample.HeadCut - sampleStats.Sample.TailCut
	stats.TotalRefLengthGoodAligned += trimmedLen * sampleStats.GoodAlignedReads
	stats.TotalAlignedBases += sampleStats.AlignedBases
}
