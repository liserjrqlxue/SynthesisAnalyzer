package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// 创建合并的参考序列文件（用于污染检测）
func (a *AlignmentAnalyzer) createMergedReferenceFile() (string, error) {
	refDir := filepath.Join(a.outputDir, "references")
	mergedRefFile := filepath.Join(refDir, "merged_all.fasta")

	// 创建合并参考序列
	var content strings.Builder
	for _, sample := range a.samples {
		if len(sample.FullReference) == 0 {
			continue
		}
		content.WriteString(fmt.Sprintf(">%s\n%s\n", sample.Name, sample.FullReference))
	}

	if err := os.WriteFile(mergedRefFile, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("创建合并参考序列文件失败: %v", err)
	}

	return mergedRefFile, nil
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

// 执行交叉污染检测
func (a *AlignmentAnalyzer) performContaminationDetection() error {
	fmt.Println("\n=== 执行交叉污染检测 ===")

	// 1. 创建合并参考序列
	mergedRefFile, err := a.createMergedReferenceFile()
	if err != nil {
		return fmt.Errorf("创建合并参考序列失败: %v", err)
	}
	fmt.Printf("  合并参考序列已创建: %s\n", filepath.Base(mergedRefFile))

	// 2. 为每个样品执行比对
	contaminationMatrix := make(map[string]map[string]int64)
	unmappedCounts := make(map[string]int64)
	totalCounts := make(map[string]int64)

	// 初始化矩阵
	for _, sample := range a.samples {
		contaminationMatrix[sample.Name] = make(map[string]int64)
		for _, targetSample := range a.samples {
			contaminationMatrix[sample.Name][targetSample.Name] = 0
		}
		unmappedCounts[sample.Name] = 0
		totalCounts[sample.Name] = 0
	}

	// 3. 对每个样品的拆分数据进行比对
	for _, sample := range a.samples {
		fmt.Printf("  分析样品: %s\n", sample.Name)

		// 获取拆分后的fastq文件
		splitFastq := filepath.Join(sample.OutputPath, "split_reads.fastq.gz")
		if _, err := os.Stat(splitFastq); os.IsNotExist(err) {
			fmt.Printf("  警告: 样品 %s 的拆分文件不存在，跳过\n", sample.Name)
			continue
		}

		// 生成输出bam文件路径
		contamBam := filepath.Join(sample.OutputPath, "contamination.bam")

		// 构建minimap2命令
		cmd := fmt.Sprintf("minimap2 -a -x sr -t %d --secondary=no %s %s | samtools view -bS -F 4 -q 10 | samtools sort -o %s",
			a.config.AlignerThreads, mergedRefFile, splitFastq, contamBam)

		// 执行命令
		if err := runCommand(cmd); err != nil {
			fmt.Printf("  警告: 比对失败: %v\n", err)
			continue
		}

		// 构建samtools命令统计每个参考序列的比对数
		samtoolsCmd := fmt.Sprintf("samtools idxstats %s", contamBam)
		output, err := runCommandWithOutput(samtoolsCmd)
		if err != nil {
			fmt.Printf("  警告: 统计比对结果失败: %v\n", err)
			continue
		}

		// 解析samtools输出
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			reference := parts[0]
			count, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				continue
			}

			if reference == "*" {
				// 未比对的reads
				unmappedCounts[sample.Name] = count
			} else {
				// 比对到特定样品
				contaminationMatrix[sample.Name][reference] = count
				totalCounts[sample.Name] += count
			}
		}

		// 清理临时文件
		os.Remove(contamBam)
		os.Remove(contamBam + ".bai")
	}

	// 4. 生成污染矩阵报告
	if err := a.generateContaminationMatrixReport(contaminationMatrix, unmappedCounts, totalCounts); err != nil {
		return fmt.Errorf("生成污染矩阵报告失败: %v", err)
	}

	return nil
}

// 生成污染矩阵报告
func (a *AlignmentAnalyzer) generateContaminationMatrixReport(matrix map[string]map[string]int64, unmapped map[string]int64, total map[string]int64) error {
	reportDir := filepath.Join(a.outputDir, "reports")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return err
	}

	// 生成矩阵文件
	matrixFile := filepath.Join(reportDir, "contamination_matrix.csv")
	f, err := os.Create(matrixFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{"Sample"}
	for _, sample := range a.samples {
		header = append(header, sample.Name)
	}
	header = append(header, "Unmapped", "Total", "Mapping_Rate")
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		record := []string{sample.Name}
		totalReads := total[sample.Name] + unmapped[sample.Name]
		mappingRate := 0.0
		if totalReads > 0 {
			mappingRate = float64(total[sample.Name]) / float64(totalReads)
		}

		for _, targetSample := range a.samples {
			count := matrix[sample.Name][targetSample.Name]
			percentage := 0.0
			if totalReads > 0 {
				percentage = float64(count) / float64(totalReads) * 100
			}
			record = append(record, fmt.Sprintf("%d (%.2f%%)", count, percentage))
		}

		unmappedPercentage := 0.0
		if totalReads > 0 {
			unmappedPercentage = float64(unmapped[sample.Name]) / float64(totalReads) * 100
		}

		record = append(record,
			fmt.Sprintf("%d (%.2f%%)", unmapped[sample.Name], unmappedPercentage),
			fmt.Sprintf("%d", totalReads),
			fmt.Sprintf("%.2f%%", mappingRate*100))

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	fmt.Printf("污染矩阵报告已生成: %s\n", matrixFile)
	return nil
}

// 执行命令并返回输出
func runCommandWithOutput(cmd string) (string, error) {
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// 执行命令
func runCommand(cmd string) error {
	_, err := exec.Command("sh", "-c", cmd).Output()
	return err
}
