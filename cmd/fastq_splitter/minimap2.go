package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// 使用minimap2进行比对
func (a *AlignmentAnalyzer) runAlignment() error {
	fmt.Println("\n=== 运行序列比对 ===")

	a.stats.startTime = time.Now()
	a.stats.totalSamples = len(a.samples)

	// 创建比对结果目录
	alignDir := filepath.Join(a.outputDir, "alignment")
	if err := os.MkdirAll(alignDir, 0755); err != nil {
		return fmt.Errorf("创建比对目录失败: %v", err)
	}

	// 并行处理每个样品
	var wg sync.WaitGroup
	sem := make(chan struct{}, a.config.AlignerThreads)

	results := make(chan *AlignmentResult, len(a.samples))

	for _, sample := range a.samples {
		// 检查输入文件是否存在
		inputFile := filepath.Join(sample.OutputPath, "target_only_reads.fastq.gz")
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

		go func(s *SampleInfo) {
			defer func() {
				<-sem
				wg.Done()
			}()

			result, err := a.alignSample(s)
			if err != nil {
				fmt.Printf("  样本 %s 比对失败: %v\n", s.Name, err)
				results <- &AlignmentResult{Sample: s, Error: err}
				return
			}

			results <- &AlignmentResult{Sample: s, Alignment: result}
		}(sample)
	}

	// 等待所有比对完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	successful := 0
	failed := 0

	for result := range results {
		if result.Error != nil {
			failed++
			continue
		}

		successful++
		a.stats.processedSamples++

		// 更新样品信息
		result.Sample.AlignmentResult = result.Alignment
		result.Sample.BamFile = result.Alignment.BamFile
		result.Sample.PositionStats = result.Alignment.PositionStats

		fmt.Printf("  样本 %s: 比对完成 (%d reads, %.1f%% mapping rate)\n",
			result.Sample.Name,
			result.Alignment.Summary.MappedReads,
			result.Alignment.Summary.MappingRate)
	}

	a.stats.endTime = time.Now()

	fmt.Printf("\n比对完成: %d 个成功, %d 个失败\n", successful, failed)
	fmt.Printf("总耗时: %v\n", a.stats.endTime.Sub(a.stats.startTime))

	return nil
}

// 单个样品的比对
func (a *AlignmentAnalyzer) alignSample(sample *SampleInfo) (*SampleAlignment, error) {
	// 创建样品特定的输出目录
	sampleAlignDir := filepath.Join(a.outputDir, "alignment", sample.Name)
	if err := os.MkdirAll(sampleAlignDir, 0755); err != nil {
		return nil, fmt.Errorf("创建比对目录失败: %v", err)
	}

	// 1. 运行minimap2
	samFile := filepath.Join(sampleAlignDir, fmt.Sprintf("%s.sam", sample.Name))
	cmd := exec.Command("minimap2",
		"-a",            // 输出SAM格式
		"-x", "map-ont", // 针对合成序列的预设
		"-t", fmt.Sprintf("%d", a.config.AlignerThreads/len(a.samples)+1),
		"--secondary=no", // 不输出secondary比对
		"-o", samFile,
		sample.ReferenceFile,
		filepath.Join(sample.OutputPath, "target_only_reads.fastq.gz"),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("minimap2执行失败: %v\n%s", err, stderr.String())
	}

	// 2. 转换SAM为BAM并排序
	bamFile := filepath.Join(sampleAlignDir, fmt.Sprintf("%s.sorted.bam", sample.Name))

	// 使用samtools view + sort
	cmd1 := exec.Command("samtools", "view", "-bS", samFile)
	cmd2 := exec.Command("samtools", "sort", "-o", bamFile)

	// 管道连接
	cmd2.Stdin, _ = cmd1.StdoutPipe()
	var stderr2 bytes.Buffer
	cmd2.Stderr = &stderr2

	if err := cmd2.Start(); err != nil {
		return nil, fmt.Errorf("启动samtools失败: %v", err)
	}

	if err := cmd1.Run(); err != nil {
		return nil, fmt.Errorf("samtools view失败: %v", err)
	}

	if err := cmd2.Wait(); err != nil {
		return nil, fmt.Errorf("samtools sort失败: %v\n%s", err, stderr2.String())
	}

	// 3. 索引BAM文件
	indexCmd := exec.Command("samtools", "index", bamFile)
	if err := indexCmd.Run(); err != nil {
		return nil, fmt.Errorf("索引BAM文件失败: %v", err)
	}

	// 4. 分析比对结果
	alignment, err := a.analyzeBamFile(bamFile, sample)
	if err != nil {
		return nil, fmt.Errorf("分析BAM文件失败: %v", err)
	}

	// 5. 清理临时文件（可选）
	if !a.config.KeepBamFiles {
		os.Remove(samFile)
	}

	return alignment, nil
}
