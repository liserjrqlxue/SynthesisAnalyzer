package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// 创建输出目录结构
func (s *EnhancedSplitter) createOutputDirs() error {
	// 创建主输出目录
	if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 创建合并文件目录
	mergedDir := filepath.Join(s.config.OutputDir, "merged")
	if err := os.MkdirAll(mergedDir, 0755); err != nil {
		return fmt.Errorf("创建合并文件目录失败: %v", err)
	}

	// 创建日志目录
	logDir := filepath.Join(s.config.OutputDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 为每个样品创建输出目录
	for _, sample := range s.samples {
		if err := os.MkdirAll(sample.OutputPath, 0755); err != nil {
			return fmt.Errorf("创建样品目录失败(%s): %v", sample.Name, err)
		}
	}

	fmt.Printf("输出目录结构创建完成: %s\n", s.config.OutputDir)
	return nil
}

// 检查文件是否存在
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// 添加缺失的辅助函数
func (s *EnhancedSplitter) getSuccessSplitCount() int {
	count := 0
	for _, sample := range s.samples {
		if sample.MatchedReads > 0 {
			count++
		}
	}
	return count
}

func (s *EnhancedSplitter) getAverageSplitRate() float64 {
	totalRate := 0.0
	count := 0
	for _, sample := range s.samples {
		if sample.TotalReads > 0 {
			totalRate += float64(sample.MatchedReads) / float64(sample.TotalReads) * 100
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return totalRate / float64(count)
}

func (s *EnhancedSplitter) getAlignedSampleCount() int {
	count := 0
	for _, sample := range s.samples {
		if sample.AlignmentResult != nil {
			count++
		}
	}
	return count
}

func (s *EnhancedSplitter) getAverageCoverage() float64 {
	totalCoverage := 0.0
	count := 0
	for _, sample := range s.samples {
		if sample.AlignmentResult != nil && sample.AlignmentResult.Summary != nil {
			totalCoverage += sample.AlignmentResult.Summary.AverageCoverage
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return totalCoverage / float64(count)
}

func (s *EnhancedSplitter) getAverageSuccessRate() float64 {
	totalSuccess := 0.0
	count := 0
	for _, sample := range s.samples {
		if sample.AlignmentResult != nil && sample.AlignmentResult.Summary != nil {
			totalSuccess += sample.AlignmentResult.Summary.SynthesisSuccess
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return totalSuccess / float64(count)
}
