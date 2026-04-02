package splitter

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"SynthesisAnalyzer/pkg/cfg"
)

// 比对分析器
type AlignmentAnalyzer struct {
	Config  *cfg.Config
	samples []*cfg.Sample

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

	fmt.Printf("\r参考序列文件创建完成: %d 个文件\n", createdCount)
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
		fmt.Fprintf(&content, ">%s\n%s\n", sample.Name, sample.FullReference)
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
	}
	for i := range cfg.ReadTypeOther {
		header = append(header, cfg.ReadTypeNames[i])
	}
	header = append(header, "High_Error_Positions")

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
		}
		for i := range cfg.ReadTypeOther {
			record = append(record, fmt.Sprintf("%d", summary.ReadTypeCounts[i]))
		}
		record = append(record, errorPositions)

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
		if err := sample.GeneratePositionReport(); err != nil {
			return err
		}

		// 生成错误率分布图数据
		if err := sample.GenerateErrorDistribution(); err != nil {
			return err
		}
	}

	return nil
}

// 运行污染检测的并行比对
func (a *AlignmentAnalyzer) runContaminationAlignment(mergedRefFile string) (map[string]string, error) {
	fmt.Println("\n=== 运行污染检测比对 ===")

	// 并行处理每个样品
	var wg sync.WaitGroup
	sem := make(chan struct{}, a.Config.Threads)

	bamFiles := make(chan struct {
		sampleName string
		bamFile    string
		err        error
	}, len(a.samples))

	for _, sample := range a.samples {
		// 检查输入文件是否存在
		inputFile := filepath.Join(sample.OutputDir, "target_only_reads.fastq.gz")
		if _, err := os.Stat(inputFile); os.IsNotExist(err) {
			fmt.Printf("  警告: 样本 %s 的输入文件不存在，跳过\n", sample.Name)
			slog.Warn("拆分文件不存在，跳过\n", "Name", sample.Name, "path", inputFile)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(s *cfg.Sample) {
			defer func() {
				fmt.Print(".")
				<-sem
				wg.Done()
			}()

			// 使用通用比对方法进行污染检测比对
			bamFile, err := AlignSampleWithParams(s.OutputDir, mergedRefFile, "contamination", max(a.Config.Threads/len(a.samples), 1))
			if err != nil {
				slog.Error("污染检测比对失败", "样本", s.Name, "err", err)
			}

			// 返回生成的BAM文件路径
			bamFiles <- struct {
				sampleName string
				bamFile    string
				err        error
			}{s.Name, bamFile, err}
		}(sample)
	}

	// 等待所有比对完成
	go func() {
		wg.Wait()
		fmt.Println()
		close(bamFiles)
	}()

	// 收集结果
	result := make(map[string]string)
	successful := 0
	failed := 0

	for bamResult := range bamFiles {
		if bamResult.err != nil {
			failed++
			continue
		}

		successful++
		result[bamResult.sampleName] = bamResult.bamFile
	}

	fmt.Printf("  污染检测比对完成: %d 成功, %d 失败\n", successful, failed)
	return result, nil
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

	// 2. 对每个样品的拆分数据进行比对
	bamFiles, err := a.runContaminationAlignment(mergedRefFile)

	// 3. 分析每个样品的比对结果
	contaminationMatrix, unmappedCounts, totalCounts, err := a.analyzeContamination(bamFiles)

	// 4. 生成污染矩阵报告
	if err := a.generateContaminationMatrixReport(contaminationMatrix, unmappedCounts, totalCounts); err != nil {
		return fmt.Errorf("生成污染矩阵报告失败: %v", err)
	}

	return nil
}

// 分析污染检测比对结果
func (a *AlignmentAnalyzer) analyzeContamination(bamFiles map[string]string) (map[string]map[string]int64, map[string]int64, map[string]int64, error) {
	fmt.Println("\n=== 分析污染检测结果 ===")

	// 初始化矩阵
	contaminationMatrix := make(map[string]map[string]int64)
	unmappedCounts := make(map[string]int64)
	totalCounts := make(map[string]int64)

	// 打开污染详情文件
	contamLogFile := filepath.Join(a.outputDir, "reports", "contamination_details.txt")
	f, err := os.Create(contamLogFile)
	if err != nil {
		slog.Warn("创建污染详情文件失败", "file", contamLogFile, "err", err)
	}
	if f != nil {
		defer f.Close()
		// 写入文件头
		fmt.Fprintf(f, "样品名称\t总reads\t比对\t未比对\t正确比对\t正确比对比例\t最高污染样品\t最高污染count\t最高污染比例\t次高污染样品\t次高污染count\t次高污染比例\n")
	}

	// 分析每个样品的比对结果
	for _, sample := range a.samples {
		sampleName := sample.Name
		contaminationMatrix[sampleName] = make(map[string]int64)
		for _, targetSample := range a.samples {
			contaminationMatrix[sampleName][targetSample.Name] = 0
		}
		unmappedCounts[sampleName] = 0
		totalCounts[sampleName] = 0

		bamFile, ok := bamFiles[sample.Name]
		if !ok {
			slog.Warn("污染bam不存在,跳过", "Name", sample.Name)
			continue
		}

		// 运行 samtools idxstats
		idxstatsCmd := fmt.Sprintf("samtools idxstats %s 2>&1", bamFile)
		output, err := runCommandWithOutput(idxstatsCmd)
		if err != nil {
			slog.Error("samtools idxstats 失败", "bamFile", bamFile, "err", err, "输出", output)
			continue
		}

		// 解析 samtools 输出
		lines := strings.Split(output, "\n")
		sampleTotal := int64(0)
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
				// 未比对的 reads
				unmappedCounts[sampleName] = count
				sampleTotal += count
			} else {
				// 比对到特定样品
				contaminationMatrix[sampleName][reference] = count
				totalCounts[sampleName] += count
				sampleTotal += count
			}
		}

		// 计算最高和次高比例的样品
		type contaminationRecord struct {
			sample string
			count  int64
			ratio  float64
		}

		var contaminationRecords []contaminationRecord
		for targetSample := range contaminationMatrix[sampleName] {
			if targetSample == sampleName {
				continue // 跳过自身
			}
			count := contaminationMatrix[sampleName][targetSample]
			ratio := 0.0
			if sampleTotal > 0 {
				ratio = float64(count) / float64(sampleTotal) * 100
			}
			contaminationRecords = append(contaminationRecords, contaminationRecord{
				sample: targetSample,
				count:  count,
				ratio:  ratio,
			})
		}

		// 按比例排序
		for i := 0; i < len(contaminationRecords); i++ {
			for j := i + 1; j < len(contaminationRecords); j++ {
				if contaminationRecords[i].ratio < contaminationRecords[j].ratio {
					contaminationRecords[i], contaminationRecords[j] = contaminationRecords[j], contaminationRecords[i]
				}
			}
		}

		// 获取最高和次高污染
		highestContam := contaminationRecord{sample: "None", count: 0, ratio: 0}
		secondHighestContam := contaminationRecord{sample: "None", count: 0, ratio: 0}

		if len(contaminationRecords) > 0 {
			highestContam = contaminationRecords[0]
			if len(contaminationRecords) > 1 {
				secondHighestContam = contaminationRecords[1]
			}
		}

		// 计算自身比对比例
		selfRatio := 0.0
		if sampleTotal > 0 {
			selfRatio = float64(contaminationMatrix[sampleName][sampleName]) / float64(sampleTotal) * 100
		}

		// 输出到文件
		if f != nil {
			fmt.Fprintf(f, "%s\t%d\t%d\t%d\t%d\t%.2f%%\t%s\t%d\t%.2f%%\t%s\t%d\t%.2f%%\n",
				sampleName,
				sampleTotal,
				totalCounts[sampleName],
				unmappedCounts[sampleName],
				contaminationMatrix[sampleName][sampleName],
				selfRatio,
				highestContam.sample,
				highestContam.count,
				highestContam.ratio,
				secondHighestContam.sample,
				secondHighestContam.count,
				secondHighestContam.ratio)
		}

		// 输出到控制台
		// fmt.Printf("  样品 %s 统计: 总reads=%d, 比对=%d, 未比对=%d\n",
		// 	sampleName, sampleTotal, totalCounts[sampleName], unmappedCounts[sampleName])
		// fmt.Printf("  正确比对: %d (%.2f%%)\n", contaminationMatrix[sampleName][sampleName], selfRatio)
		// fmt.Printf("  最高污染: %s - %d (%.2f%%)\n", highestContam.sample, highestContam.count, highestContam.ratio)
		// fmt.Printf("  次高污染: %s - %d (%.2f%%)\n", secondHighestContam.sample, secondHighestContam.count, secondHighestContam.ratio)
	}

	return contaminationMatrix, unmappedCounts, totalCounts, nil
}

// 生成污染矩阵报告
func (a *AlignmentAnalyzer) generateContaminationMatrixReport(matrix map[string]map[string]int64, unmapped map[string]int64, total map[string]int64) error {
	reportDir := filepath.Join(a.outputDir, "reports")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return err
	}

	// 生成count矩阵文件
	countFile := filepath.Join(reportDir, "contamination_matrix_count.csv")
	countF, err := os.Create(countFile)
	if err != nil {
		return err
	}
	defer countF.Close()
	countWriter := csv.NewWriter(countF)
	defer countWriter.Flush()

	// 生成ratio矩阵文件
	ratioFile := filepath.Join(reportDir, "contamination_matrix_ratio.csv")
	ratioF, err := os.Create(ratioFile)
	if err != nil {
		return err
	}
	defer ratioF.Close()
	ratioWriter := csv.NewWriter(ratioF)
	defer ratioWriter.Flush()

	// 写入表头
	header := []string{"Sample"}
	for _, sample := range a.samples {
		header = append(header, a.getSampleShortName(sample.Name))
	}
	header = append(header, "Unmapped", "Total", "Mapping_Rate")

	if err := countWriter.Write(header); err != nil {
		return err
	}
	if err := ratioWriter.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		countRecord := []string{a.getSampleShortName(sample.Name)}
		ratioRecord := []string{a.getSampleShortName(sample.Name)}
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
			countRecord = append(countRecord, fmt.Sprintf("%d", count))
			ratioRecord = append(ratioRecord, fmt.Sprintf("%.2f%%", percentage))
		}

		unmappedPercentage := 0.0
		if totalReads > 0 {
			unmappedPercentage = float64(unmapped[sample.Name]) / float64(totalReads) * 100
		}

		countRecord = append(countRecord,
			fmt.Sprintf("%d", unmapped[sample.Name]),
			fmt.Sprintf("%d", totalReads),
			fmt.Sprintf("%.2f%%", mappingRate*100))

		ratioRecord = append(ratioRecord,
			fmt.Sprintf("%.2f%%", unmappedPercentage),
			fmt.Sprintf("%d", totalReads),
			fmt.Sprintf("%.2f%%", mappingRate*100))

		if err := countWriter.Write(countRecord); err != nil {
			return err
		}
		if err := ratioWriter.Write(ratioRecord); err != nil {
			return err
		}
	}

	fmt.Printf("污染矩阵报告已生成: %s, %s\n", countFile, ratioFile)
	return nil
}

// 获取样品短名称（使用"."分割后的最后一部分）
func (a *AlignmentAnalyzer) getSampleShortName(fullName string) string {
	parts := strings.Split(fullName, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullName
}

// 执行命令并返回输出
func runCommandWithOutput(cmd string) (string, error) {
	command := exec.Command("sh", "-c", cmd)
	output, err := command.CombinedOutput()
	return string(output), err
}

// 执行命令
func runCommand(cmd string) error {
	command := exec.Command("sh", "-c", cmd)
	return command.Run()
}
