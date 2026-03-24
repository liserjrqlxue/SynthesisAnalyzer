package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 比对配置
type AlignmentConfig struct {
	UseMinimap2    bool    // 使用minimap2而不是BWA
	AlignerThreads int     // 比对线程数
	MapQThreshold  int     // 比对质量阈值
	MinIdentity    float64 // 最小identity百分比
	SkipAlignment  bool    // 是否跳过比对步骤
	KeepSamFiles   bool    // 是否保留SAM文件
	AnalysisOnly   bool    // 仅分析已有的BAM文件
}

// 比对分析器
type AlignmentAnalyzer struct {
	config    *AlignmentConfig
	samples   []*SampleInfo
	outputDir string
	stats     *AlignmentStats
}

// 比对统计
type AlignmentStats struct {
	totalSamples     int
	processedSamples int
	totalReads       int64
	alignedReads     int64
	failedAlignments int64
	startTime        time.Time
	endTime          time.Time
}

// 为每个样品创建参考序列文件
func (a *AlignmentAnalyzer) createReferenceFiles() error {
	fmt.Println("\n=== 创建参考序列文件 ===")

	refDir := filepath.Join(a.outputDir, "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		return fmt.Errorf("创建参考序列目录失败: %v", err)
	}

	createdCount := 0

	for _, sample := range a.samples {
		// 构建完整的参考序列：头靶标 + 合成序列 + 尾靶标
		// fullReference := sample.TargetSeq + sample.SynthesisSeq + sample.PostTargetSeq
		if len(sample.FullReference) == 0 {
			fmt.Printf("  警告: 样本 %s 没有参考序列信息，跳过\n", sample.Name)
			continue
		}

		// 创建参考序列文件
		refFile := filepath.Join(refDir, fmt.Sprintf("%s.fasta", sample.Name))

		content := fmt.Sprintf(">%s\n%s\n", sample.Name, sample.FullReference)

		if err := os.WriteFile(refFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("创建参考序列文件失败(%s): %v", sample.Name, err)
		}

		sample.ReferenceFile = refFile
		sample.ReferenceSeq = sample.FullReference
		sample.ReferenceLen = len(sample.FullReference)

		createdCount++

		if createdCount%10 == 0 {
			fmt.Printf("\r  已创建 %d 个参考序列文件", createdCount)
		}
	}

	fmt.Printf("\n参考序列文件创建完成: %d 个文件\n", createdCount)
	return nil
}

// 生成详细的比对报告
func (a *AlignmentAnalyzer) generateAlignmentReport() error {
	fmt.Println("\n=== 生成比对分析报告 ===")

	reportDir := filepath.Join(a.outputDir, "reports")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return fmt.Errorf("创建报告目录失败: %v", err)
	}

	// 1. 生成汇总报告
	if err := a.generateSummaryReport(reportDir); err != nil {
		return err
	}

	// 2. 生成每个样品的详细报告
	if err := a.generatePerSampleReports(); err != nil {
		return err
	}

	// 3. 生成可视化数据
	if err := a.generateVisualizationData(reportDir); err != nil {
		return err
	}

	fmt.Println("比对报告生成完成")
	return nil
}

// 生成汇总报告
func (a *AlignmentAnalyzer) generateSummaryReport(reportDir string) error {
	reportFile := filepath.Join(reportDir, "alignment_summary.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"Sample",
		"Reference_Length",
		"Total_Reads",
		"Mapped_Reads",
		"Mapping_Rate",
		"Average_Coverage",
		"Average_Identity",
		"Synthesis_Success",
		"PerfectReads",
		"MismatchOnly",
		"InsertionOnly",
		"DeletionOnly",
		"MixedMismatchIns",
		"MixedMismatchDel",
		"MixedInsDel",
		"AllErrors",
		"Other",
		"High_Error_Positions",
		"BAM_File",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		summary := sample.AlignmentResult.Summary
		errorPositions := ""
		if len(summary.ErrorPositions) > 0 {
			errorPositions = strings.Trim(strings.Join(strings.Fields(fmt.Sprint(summary.ErrorPositions)), ","), "[]")
		}

		record := []string{
			sample.Name,
			fmt.Sprintf("%d", sample.ReferenceLen),
			fmt.Sprintf("%d", summary.TotalReads),
			fmt.Sprintf("%d", summary.MappedReads),
			fmt.Sprintf("%.3f", summary.MappingRate),
			fmt.Sprintf("%.3f", summary.AverageCoverage),
			fmt.Sprintf("%.3f", summary.AverageIdentity),
			fmt.Sprintf("%.3f", summary.SynthesisSuccess),
			fmt.Sprintf("%d", summary.ReadTypeCounts.PerfectReads),
			fmt.Sprintf("%d", summary.ReadTypeCounts.MismatchOnly),
			fmt.Sprintf("%d", summary.ReadTypeCounts.InsertionOnly),
			fmt.Sprintf("%d", summary.ReadTypeCounts.DeletionOnly),
			fmt.Sprintf("%d", summary.ReadTypeCounts.MixedMismatchIns),
			fmt.Sprintf("%d", summary.ReadTypeCounts.MixedMismatchDel),
			fmt.Sprintf("%d", summary.ReadTypeCounts.MixedInsDel),
			fmt.Sprintf("%d", summary.ReadTypeCounts.AllErrors),
			fmt.Sprintf("%d", summary.ReadTypeCounts.Other),
			errorPositions,
			filepath.Base(sample.BamFile),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	fmt.Printf("汇总报告已生成: %s\n", reportFile)
	return nil
}

// 生成每个样品的详细报告
func (a *AlignmentAnalyzer) generatePerSampleReports() error {
	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		// 生成位置详细统计
		if err := sample.generatePositionReport(); err != nil {
			return err
		}

		// 生成错误率分布图数据
		if err := sample.generateErrorDistribution(); err != nil {
			return err
		}
	}

	return nil
}
