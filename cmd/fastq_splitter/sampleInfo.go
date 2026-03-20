package main

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// 扩展SampleInfo结构，添加提取统计
type SampleInfo struct {
	Name          string
	TargetSeq     string // 头靶标序列
	SynthesisSeq  string // 合成序列
	PostTargetSeq string // 尾靶标序列
	R1Path        string
	R2Path        string
	OutputPath    string

	// 完整参考序列（用于分析）
	FullReference string

	// 统计信息
	TotalReads   int
	MatchedReads int
	ForwardReads int
	ReverseReads int

	// 提取序列统计
	MinExtractedLen   int
	MaxExtractedLen   int
	TotalExtractedLen int

	// 状态信息
	MergedFile string
	DoneFile   string // 完成标签文件
	Status     string
	ReadCount  int

	// 反向互补序列（用于匹配）
	ReverseTarget     string // 头靶标反向互补
	ReversePostTarget string // 尾靶标反向互补

	MergeTime time.Time
	SplitTime time.Time

	// 比对相关字段（新增）
	ReferenceFile   string           // 参考序列文件路径
	ReferenceSeq    string           // 完整参考序列
	ReferenceLen    int              // 参考序列长度
	AlignmentResult *SampleAlignment // 比对结果
	BamFile         string           // BAM文件路径
	PositionStats   []PositionStat   // 位置统计信息
}

// 生成位置详细统计
func (sample *SampleInfo) generatePositionReport() error {
	reportFile := filepath.Join(sample.OutputPath, "position_stats.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"Position",
		"Reference_Base",
		"Total_Reads",
		"Coverage",
		"Match_Count",
		"Mismatch_Count",
		"Deletion_Count",
		"Insertion_Count",
		"Error_Rate",
		"Synthesis_Success",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	alignment := sample.AlignmentResult
	for i, pos := range alignment.PositionStats {
		refBase := "N"
		if i < len(alignment.ReferenceSeq) {
			refBase = string(alignment.ReferenceSeq[i])
		}

		synthesisSuccess := 100.0
		total := pos.MatchCount + pos.MismatchCount + pos.DeletionCount + pos.InsertionCount
		if total > 0 {
			synthesisSuccess = float64(pos.MatchCount) / float64(total) * 100
		}

		record := []string{
			fmt.Sprintf("%d", pos.Position),
			refBase,
			fmt.Sprintf("%d", pos.TotalReads),
			fmt.Sprintf("%.4f", pos.Coverage),
			fmt.Sprintf("%d", pos.MatchCount),
			fmt.Sprintf("%d", pos.MismatchCount),
			fmt.Sprintf("%d", pos.DeletionCount),
			fmt.Sprintf("%d", pos.InsertionCount),
			fmt.Sprintf("%.4f", pos.ErrorRate),
			fmt.Sprintf("%.2f", synthesisSuccess),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	// fmt.Printf("  样本 %s 位置统计已生成\n", sample.Name)
	slog.Debug("位置统计已生成", "样本", sample.Name)
	return nil
}

// 添加缺失的比对分析器函数
func (sample *SampleInfo) generateErrorDistribution() error {
	// 生成错误率分布数据
	reportFile := filepath.Join(sample.OutputPath, "error_distribution.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{"Position", "ErrorRate"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	alignment := sample.AlignmentResult
	for _, pos := range alignment.PositionStats {
		record := []string{
			fmt.Sprintf("%d", pos.Position),
			fmt.Sprintf("%.6f", pos.ErrorRate),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
