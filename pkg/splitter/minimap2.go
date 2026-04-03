package splitter

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"SynthesisAnalyzer/pkg/cfg"
)

// 使用minimap2进行比对
func (a *AlignmentAnalyzer) runAlignment() error {
	fmt.Println("\n=== 运行序列比对 ===")
	var (
		startTime = time.Now()
		// 处理结果
		total      = len(a.Samples)
		successful = 0
		failed     = 0
	)
	defer func() {
		fmt.Printf("比对完成: %d 个样本, %d 个成功, %d 个失败\n", total, successful, failed)
		fmt.Printf("总耗时: %v\n", time.Since(startTime))
	}()

	// 并行处理每个样品
	var wg sync.WaitGroup
	sem := make(chan struct{}, a.Config.Threads)
	singleThread := max(a.Config.Threads/total, 1)

	results := make(chan *AlignmentResult, total)

	for _, sample := range a.Samples {
		// 检查输入文件是否存在
		inputFile := filepath.Join(sample.OutputDir, "target_only_reads.fastq.gz")
		if _, err := os.Stat(inputFile); os.IsNotExist(err) {
			fmt.Printf("  警告: 样本 %s 的输入文件不存在，跳过\n", sample.Name)
			continue
		}

		// 检查参考序列文件
		if sample.ReferenceFile == "" {
			fmt.Printf("  警告: 样本 %s 没有参考序列文件，跳过\n", sample.Name)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(s *cfg.Sample) {
			defer func() {
				<-sem
				wg.Done()
			}()

			result := alignSample(s, singleThread, a.Config.Alignment.MapQThreshold)
			if result.Error != nil {
				slog.Error("比对失败", "样本", s.Name, "err", result.Error)
			}
			results <- result
		}(sample)
	}

	// 等待所有比对完成
	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.Error != nil {
			failed++
			continue
		}

		successful++

		// 更新样品信息
		result.Sample.AlignmentResult = result.Alignment
		result.Sample.PositionStats = result.Alignment.PositionStats

		slog.Debug("比对完成",
			"样本", result.Sample.Name,
			"映射读数", result.Alignment.Summary.MappedReads,
			"映射率", result.Alignment.Summary.MappingRate)
	}

	if successful < total {
		return fmt.Errorf("比对失败，有 %d 个样本未成功比对", total-successful)
	}

	return nil
}

// 单个样品的比对
func alignSample(sample *cfg.Sample, threads, mapQThreshold int) *AlignmentResult {
	var result = &AlignmentResult{Sample: sample}
	// 调用通用的比对方法
	bamFile, err := AlignSampleWithParams(
		sample.OutputDir,
		sample.ReferenceFile,
		sample.Name,
		threads,
	)
	sample.BamFile = bamFile
	if err != nil {
		result.Error = err
		return result
	}

	// 分析比对结果
	alignment, err := analyzeBamFile(bamFile, sample, mapQThreshold)
	result.Error = err
	result.Alignment = alignment
	return result
}

func AlignSampleWithParams(workDir, referenceFile, outputPrefix string, threads int) (string, error) {
	var (
		fastqFile = filepath.Join(workDir, "target_only_reads.fastq.gz")

		samFile  = filepath.Join(workDir, fmt.Sprintf("%s.sam", outputPrefix))
		bamFile  = filepath.Join(workDir, fmt.Sprintf("%s.sorted.bam", outputPrefix))
		doneFile = bamFile + ".done"
	)
	// 创建样品特定的输出目录
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("创建比对目录失败: %v", err)
	}

	// 检查是否已完成比对
	if _, err := os.Stat(doneFile); err == nil {
		// 检查BAM文件是否存在
		if _, err := os.Stat(bamFile); err == nil {
			slog.Debug("比对已完成，跳过", "prefix", outputPrefix)
			return bamFile, nil
		}
	}

	// 检查输入文件是否存在
	if _, err := os.Stat(fastqFile); os.IsNotExist(err) {
		return "", fmt.Errorf("输入文件不存在: %s", fastqFile)
	}

	// 1. 运行minimap2
	cmd := exec.Command("minimap2",
		"-a", // 输出SAM格式
		"-x", "sr",
		"-z", "800",
		"--end-bonus=100",
		"--eqx",
		"--MD",
		"-t", strconv.Itoa(threads),
		"--secondary=no", // 不输出secondary比对
		"-o", samFile,
		referenceFile,
		fastqFile,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("minimap2执行失败: %v\n%s", err, stderr.String())
	}

	// 2. 转换SAM为BAM并排序

	// 使用samtools view + sort
	cmd1 := exec.Command("samtools", "view", "-bS", samFile)
	cmd2 := exec.Command("samtools", "sort", "-o", bamFile)

	// 管道连接
	cmd2.Stdin, _ = cmd1.StdoutPipe()
	var stderr2 bytes.Buffer
	cmd2.Stderr = &stderr2

	if err := cmd2.Start(); err != nil {
		return "", fmt.Errorf("启动samtools失败: %v", err)
	}

	if err := cmd1.Run(); err != nil {
		return "", fmt.Errorf("samtools view失败: %v", err)
	}

	if err := cmd2.Wait(); err != nil {
		return "", fmt.Errorf("samtools sort失败: %v\n%s", err, stderr2.String())
	}

	// 3. 索引BAM文件
	indexCmd := exec.Command("samtools", "index", bamFile)
	if err := indexCmd.Run(); err != nil {
		return bamFile, fmt.Errorf("索引BAM文件失败: %v", err)
	}

	os.Remove(samFile)

	// 5. 创建完成标签
	doneContent := fmt.Sprintf("Created: %s\nReference: %s\nOutput: %s\n",
		time.Now().Format(time.RFC3339),
		filepath.Base(referenceFile),
		filepath.Base(bamFile))
	if err := os.WriteFile(doneFile, []byte(doneContent), 0644); err != nil {
		fmt.Printf("  警告: 创建完成标签失败: %v\n", err)
	}

	return bamFile, nil
}
