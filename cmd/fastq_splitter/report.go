package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 生成所有报告
func (s *EnhancedSplitter) generateReports() error {
	// 1. 样品报告
	if err := s.generateSampleReport(); err != nil {
		return err
	}

	// 2. 文件报告
	if err := s.generateFileReport(); err != nil {
		return err
	}

	// 3. 汇总报告
	if err := s.generateSummaryReport(); err != nil {
		return err
	}

	return nil
}

// 生成汇总报告
func (s *EnhancedSplitter) generateSummaryReport() error {
	reportFile := filepath.Join(s.config.OutputDir, "split_summary.txt")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	// 计算统计信息
	totalReads := 0
	totalMatched := 0
	totalSamples := len(s.samples)
	totalFiles := len(s.mergedFiles)

	for _, mergedInfo := range s.mergedFiles {
		totalReads += mergedInfo.TotalReads
		totalMatched += mergedInfo.MatchedReads
	}

	overallRate := 0.0
	if totalReads > 0 {
		overallRate = float64(totalMatched) / float64(totalReads) * 100
	}

	// 写入报告头部
	summary := fmt.Sprintf(`FASTQ拆分汇总报告
=====================

生成时间: %s
程序版本: v1.0

运行配置
--------
Excel文件: %s
输出目录: %s
线程数: %d
启用反向互补: %v
质量阈值: %d
最小合并长度: %d
跳过已存在文件: %v
压缩输出: %v

总体统计
--------
合并文件数: %d
样品总数: %d
总处理reads数: %d
成功匹配reads数: %d (%.1f%%)
匹配失败reads数: %d (%.1f%%)

样品统计
--------
`,
		time.Now().Format("2006-01-02 15:04:05"),
		s.config.ExcelFile,
		s.config.OutputDir,
		s.config.Threads,
		s.config.UseRC,
		s.config.Quality,
		s.config.MergeLen,
		s.config.SkipExisting,
		s.config.Compression,
		totalFiles,
		totalSamples,
		totalReads,
		totalMatched, overallRate,
		totalReads-totalMatched, float64(totalReads-totalMatched)/float64(totalReads)*100)

	writer.WriteString(summary)

	// 写入每个样品的统计
	for i, sample := range s.samples {
		matchRate := 0.0
		if sample.TotalReads > 0 {
			matchRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		sampleInfo := fmt.Sprintf("%d. 样品: %s\n", i+1, sample.Name)
		sampleInfo += fmt.Sprintf("   头靶标: %s (长度: %d)\n", sample.TargetSeq, len(sample.TargetSeq))
		sampleInfo += fmt.Sprintf("   尾靶标: %s (长度: %d)\n", sample.PostTargetSeq, len(sample.PostTargetSeq))
		sampleInfo += fmt.Sprintf("   合成序列长度: %d\n", len(sample.SynthesisSeq))
		sampleInfo += fmt.Sprintf("   总参考长度: %d\n", len(sample.TargetSeq)+len(sample.SynthesisSeq)+len(sample.PostTargetSeq))
		sampleInfo += fmt.Sprintf("   处理reads数: %d\n", sample.TotalReads)
		sampleInfo += fmt.Sprintf("   匹配reads数: %d (%.1f%%)\n", sample.MatchedReads, matchRate)
		sampleInfo += fmt.Sprintf("   输出文件: %s/target_only_reads.fastq.gz\n", sample.OutputPath)
		sampleInfo += fmt.Sprintf("   来源文件: %s + %s\n", filepath.Base(sample.R1Path), filepath.Base(sample.R2Path))

		if sample.MergedFile != "" {
			sampleInfo += fmt.Sprintf("   合并文件: %s\n", filepath.Base(sample.MergedFile))
		}
		sampleInfo += "\n"

		writer.WriteString(sampleInfo)
	}

	// 写入文件统计
	writer.WriteString("文件统计\n--------\n")
	for i, mergedInfo := range s.mergedFiles {
		fileMatchRate := 0.0
		if mergedInfo.TotalReads > 0 {
			fileMatchRate = float64(mergedInfo.MatchedReads) / float64(mergedInfo.TotalReads) * 100
		}

		fileInfo := fmt.Sprintf("%d. 文件: %s\n", i+1, filepath.Base(mergedInfo.FilePath))
		fileInfo += fmt.Sprintf("   样品数: %d\n", len(mergedInfo.Samples))
		fileInfo += fmt.Sprintf("   样品列表: %s\n", strings.Join(mergedInfo.SampleNames, ", "))
		fileInfo += fmt.Sprintf("   总处理reads数: %d\n", mergedInfo.TotalReads)
		fileInfo += fmt.Sprintf("   匹配reads数: %d (%.1f%%)\n", mergedInfo.MatchedReads, fileMatchRate)
		fileInfo += "   状态: 已完成\n\n"

		writer.WriteString(fileInfo)
	}

	// 写入性能统计
	if !s.stats.startTime.IsZero() && !s.stats.endTime.IsZero() {
		duration := s.stats.endTime.Sub(s.stats.startTime)

		performance := fmt.Sprintf(`性能统计
--------
开始时间: %s
结束时间: %s
总运行时间: %v
平均处理速度: %.1f reads/秒

输出文件结构
------------
%s/
├── logs/                     # 日志文件
├── merged/                   # 合并后的FASTQ文件
├── sample1/                  # 样品1目录
│   └── target_only_reads.fastq.gz  # 提取的靶标序列
├── sample2/                  # 样品2目录
│   └── target_only_reads.fastq.gz
├── sample_split_report.csv   # 样品详细报告
├── file_split_report.csv     # 文件处理报告
└── split_summary.txt         # 本汇总报告

注意事项
--------
1. 所有输出文件均为gzip压缩格式
2. 提取的序列仅包含头尾靶标之间的区域
3. 反向互补匹配的序列已被转换为正向序列
4. 质量值与提取的序列对应

报告结束
`,
			s.stats.startTime.Format("2006-01-02 15:04:05"),
			s.stats.endTime.Format("2006-01-02 15:04:05"),
			duration,
			float64(totalReads)/duration.Seconds(),
			s.config.OutputDir)

		writer.WriteString(performance)
	}

	fmt.Printf("汇总报告已生成: %s\n", reportFile)
	return nil
}

// 生成文件报告
func (s *EnhancedSplitter) generateFileReport() error {
	reportFile := filepath.Join(s.config.OutputDir, "file_split_report.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"合并文件",
		"样品数",
		"样品列表",
		"总处理reads",
		"匹配reads",
		"匹配率%",
		"状态",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, mergedInfo := range s.mergedFiles {
		matchRate := 0.0
		if mergedInfo.TotalReads > 0 {
			matchRate = float64(mergedInfo.MatchedReads) / float64(mergedInfo.TotalReads) * 100
		}

		sampleList := strings.Join(mergedInfo.SampleNames, ";")

		record := []string{
			filepath.Base(mergedInfo.FilePath),
			fmt.Sprintf("%d", len(mergedInfo.Samples)),
			sampleList,
			fmt.Sprintf("%d", mergedInfo.TotalReads),
			fmt.Sprintf("%d", mergedInfo.MatchedReads),
			fmt.Sprintf("%.1f", matchRate),
			mergedInfo.Status,
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	fmt.Printf("文件报告已生成: %s\n", reportFile)
	return nil
}

// 生成详细的样品报告
func (s *EnhancedSplitter) generateSampleReport() error {
	reportFile := filepath.Join(s.config.OutputDir, "sample_split_report.csv")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"样品名称",
		"头靶标序列",
		"尾靶标序列",
		"合成序列长度",
		"总参考长度",
		"来源R1文件",
		"来源R2文件",
		"合并文件",
		"输出文件",
		"总处理reads",
		"匹配reads",
		"匹配率%",
		"最短提取长度",
		"最长提取长度",
		"平均提取长度",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range s.samples {
		matchRate := 0.0
		avgLength := 0.0

		if sample.TotalReads > 0 {
			matchRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		if sample.MatchedReads > 0 {
			avgLength = float64(sample.TotalExtractedLen) / float64(sample.MatchedReads)
		}

		// 确定合并文件
		mergedFile := "unknown"
		if sample.MergedFile != "" {
			mergedFile = filepath.Base(sample.MergedFile)
		}

		record := []string{
			sample.Name,
			sample.TargetSeq,
			sample.PostTargetSeq,
			fmt.Sprintf("%d", len(sample.SynthesisSeq)),
			fmt.Sprintf("%d", len(sample.FullReference)),
			filepath.Base(sample.R1Path),
			filepath.Base(sample.R2Path),
			filepath.Base(sample.MergedFile),
			mergedFile,
			filepath.Join(sample.Name, "target_only_reads.fastq.gz"),
			fmt.Sprintf("%d", sample.TotalReads),
			fmt.Sprintf("%d", sample.MatchedReads),
			fmt.Sprintf("%.1f", matchRate),
			fmt.Sprintf("%d", sample.MinExtractedLen),
			fmt.Sprintf("%d", sample.MaxExtractedLen),
			fmt.Sprintf("%.1f", avgLength),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}
	fmt.Printf("样品报告已生成: %s\n", reportFile)

	// 生成详细分析报告
	return s.generateDetailedAnalysis()
}

// 生成详细分析报告
func (s *EnhancedSplitter) generateDetailedAnalysis() error {

	// 生成汇总统计
	analysisFile := filepath.Join(s.config.OutputDir, "detailed_analysis.txt")
	reportFile := filepath.Join(s.config.OutputDir, "extraction_report.csv")
	f, err := os.Create(analysisFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	// 写入分析报告
	report := fmt.Sprintf(`靶标提取分析报告
生成时间: %s
Excel文件: %s
输出目录: %s

样品统计:
========
总样品数: %d
启用反向互补匹配: %v
允许错配数: %d
匹配分数阈值: %d
使用的线程数: %d

总体统计:
========
总处理序列数: %d
成功提取序列数: %d
总体提取率: %.1f%%

质量过滤:
========
最低质量值: %d
最小合并长度: %d

输出文件:
========
报告文件: %s
汇总文件: %s
每个样品的拆分结果在各自子目录中

处理完成!


各样本详细统计:
==============
`,
		time.Now().Format("2006-01-02 15:04:05"),
		s.config.ExcelFile,
		s.config.OutputDir,
		len(s.samples),
		s.config.UseRC,
		s.config.AllowMismatch,
		s.config.MatchThreshold,
		s.config.Threads,
		s.getTotalReads(),
		s.getTotalMatched(),
		s.getExtractionRate(),
		s.config.Quality,
		s.config.MergeLen,
		reportFile,
		analysisFile,
	)

	writer.WriteString(report)

	// 写入每个样本的详细信息
	for _, sample := range s.samples {
		avgLength := 0.0
		if sample.MatchedReads > 0 {
			avgLength = float64(sample.TotalExtractedLen) / float64(sample.MatchedReads)
		}

		sampleInfo := fmt.Sprintf(`
样本: %s
------------------------------------------------
头靶标: %s (长度: %d)
尾靶标: %s (长度: %d)
合成序列长度: %d
总参考序列长度: %d

处理统计:
  总处理序列: %d
  成功提取: %d (%.1f%%)
  提取序列长度范围: %d - %d bp
  平均提取长度: %.1f bp
  
输出文件: %s/target_only_reads.fastq.gz
`,
			sample.Name,
			sample.TargetSeq,
			len(sample.TargetSeq),
			sample.PostTargetSeq,
			len(sample.PostTargetSeq),
			len(sample.SynthesisSeq),
			len(sample.TargetSeq)+len(sample.SynthesisSeq)+len(sample.PostTargetSeq),
			sample.TotalReads,
			sample.MatchedReads,
			float64(sample.MatchedReads)/float64(sample.TotalReads)*100,
			sample.MinExtractedLen,
			sample.MaxExtractedLen,
			avgLength,
			sample.OutputPath)

		writer.WriteString(sampleInfo)
	}

	fmt.Printf("详细分析报告已生成: %s\n", analysisFile)

	return nil
}

// 辅助统计函数
func (s *EnhancedSplitter) getTotalReads() int {
	total := 0
	for _, sample := range s.samples {
		total += sample.TotalReads
	}
	return total
}

func (s *EnhancedSplitter) getTotalMatched() int {
	total := 0
	for _, sample := range s.samples {
		total += sample.MatchedReads
	}
	return total
}

func (s *EnhancedSplitter) getExtractionRate() float64 {
	totalReads := s.getTotalReads()
	totalMatched := s.getTotalMatched()

	if totalReads == 0 {
		return 0.0
	}
	return float64(totalMatched) / float64(totalReads) * 100
}

// 计算样品统计
func (s *EnhancedSplitter) calculateSampleStats() map[string]*SampleStat {
	stats := make(map[string]*SampleStat)

	for _, sample := range s.samples {
		stat := &SampleStat{
			Name:         sample.Name,
			TotalReads:   sample.TotalReads,
			MatchedReads: sample.MatchedReads,
		}

		if sample.TotalReads > 0 {
			stat.MatchRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		stats[sample.Name] = stat
	}

	return stats
}

// 样品统计结构
type SampleStat struct {
	Name         string
	TotalReads   int
	MatchedReads int
	MatchRate    float64
}

// 计算文件统计
func (s *EnhancedSplitter) calculateFileStats() map[string]*FileStat {
	stats := make(map[string]*FileStat)

	for _, mergedInfo := range s.mergedFiles {
		stat := &FileStat{
			FileName:     filepath.Base(mergedInfo.FilePath),
			SampleCount:  len(mergedInfo.Samples),
			TotalReads:   mergedInfo.TotalReads,
			MatchedReads: mergedInfo.MatchedReads,
			Status:       mergedInfo.Status,
		}

		if mergedInfo.TotalReads > 0 {
			stat.MatchRate = float64(mergedInfo.MatchedReads) / float64(mergedInfo.TotalReads) * 100
		}

		stats[mergedInfo.FilePath] = stat
	}

	return stats
}

// 文件统计结构
type FileStat struct {
	FileName     string
	SampleCount  int
	TotalReads   int
	MatchedReads int
	MatchRate    float64
	Status       string
}

// 生成最终汇总报告
func (s *EnhancedSplitter) generateFinalSummary() error {
	summaryFile := filepath.Join(s.config.OutputDir, "FINAL_SUMMARY.txt")
	f, err := os.Create(summaryFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	summary := fmt.Sprintf(`最终处理汇总报告
===================

处理完成时间: %s

目录结构:
%s/
├── merged/               # 合并的FASTQ文件
├── sample1/              # 样品1结果
│   ├── split_reads.fastq.gz      # 拆分后的序列
│   ├── target_only_reads.fastq.gz # 靶标序列
│   ├── sample1.sorted.bam
│   ├── sample1.sorted.bam.bai
│   └── position_stats.csv
├── sample2/              # 样品2结果
│   ├── split_reads.fastq.gz
│   └── target_only_reads.fastq.gz
├── references/           # 参考序列文件
├── reports/             # 分析报告
│   ├── alignment_summary.csv
│   ├── visualization/
│   │   └── synthesis_analysis.R
└── logs/               # 日志文件

处理统计:
- 总样品数: %d
- 成功拆分: %d
- 平均拆分率: %.1f%%

比对统计:
- 比对样品数: %d
- 平均覆盖度: %.2f
- 平均合成成功率: %.1f%%

后续分析建议:
1. 查看 reports/alignment_summary.csv 了解各样品统计
2. 运行 reports/visualization/synthesis_analysis.R 生成图形
3. 检查高错误率位置，可能需要优化合成条件

感谢使用FASTQ拆分与比对分析系统！
`,
		time.Now().Format("2006-01-02 15:04:05"),
		s.config.OutputDir,
		len(s.samples),
		s.getSuccessSplitCount(),
		s.getAverageSplitRate(),
		s.getAlignedSampleCount(),
		s.getAverageCoverage(),
		s.getAverageSuccessRate(),
	)

	writer.WriteString(summary)
	fmt.Printf("最终汇总报告已生成: %s\n", summaryFile)

	return nil
}
