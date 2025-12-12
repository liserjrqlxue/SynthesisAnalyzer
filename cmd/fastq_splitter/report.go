package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// 生成提取报告
func (s *RegexpSplitter) generateEnhancedReport() error {
	reportFile := filepath.Join(s.config.OutputDir, "extraction_report.csv")
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
		"头靶标长度",
		"合成序列长度",
		"尾靶标长度",
		"总参考长度",
		"R1文件",
		"R2文件",
		"合并文件",
		"输出目录",
		"处理reads数",
		"提取reads数",
		"提取率%",
		"最短提取长度",
		"最长提取长度",
		"平均提取长度",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range s.samples {
		extractionRate := 0.0
		avgLength := 0.0

		if sample.TotalReads > 0 {
			extractionRate = float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
		}

		if sample.MatchedReads > 0 {
			avgLength = float64(sample.TotalExtractedLen) / float64(sample.MatchedReads)
		}

		record := []string{
			sample.Name,
			fmt.Sprintf("%d", len(sample.TargetSeq)),
			fmt.Sprintf("%d", len(sample.SynthesisSeq)),
			fmt.Sprintf("%d", len(sample.PostTargetSeq)),
			fmt.Sprintf("%d", len(sample.FullReference)),
			filepath.Base(sample.R1Path),
			filepath.Base(sample.R2Path),
			filepath.Base(sample.MergedFile),
			sample.OutputPath,
			fmt.Sprintf("%d", sample.TotalReads),
			fmt.Sprintf("%d", sample.MatchedReads),
			fmt.Sprintf("%.1f", extractionRate),
			fmt.Sprintf("%d", sample.MinExtractedLen),
			fmt.Sprintf("%d", sample.MaxExtractedLen),
			fmt.Sprintf("%.1f", avgLength),
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}
	fmt.Printf("增强版提取报告已生成: %s\n", reportFile)

	// 生成详细分析报告
	return s.generateDetailedAnalysis()
}

// 生成详细分析报告
func (s *RegexpSplitter) generateDetailedAnalysis() error {

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
func (s *RegexpSplitter) getTotalReads() int {
	total := 0
	for _, sample := range s.samples {
		total += sample.TotalReads
	}
	return total
}

func (s *RegexpSplitter) getTotalMatched() int {
	total := 0
	for _, sample := range s.samples {
		total += sample.MatchedReads
	}
	return total
}

func (s *RegexpSplitter) getExtractionRate() float64 {
	totalReads := s.getTotalReads()
	totalMatched := s.getTotalMatched()

	if totalReads == 0 {
		return 0.0
	}
	return float64(totalMatched) / float64(totalReads) * 100
}
