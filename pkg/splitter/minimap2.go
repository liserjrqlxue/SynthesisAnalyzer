package splitter

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"SynthesisAnalyzer/pkg/cfg"
	"SynthesisAnalyzer/pkg/stats"

	"golang.org/x/sync/errgroup"
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
	a.MutationStats = stats.NewMutationStats()
	a.MutationStats.BatchInfo = new(cfg.BatchInfo{
		Config:      a.Config,
		BatchSample: a.BatchSample,
	})

	// 并行处理每个样品
	sem := make(chan struct{}, a.Config.Threads)
	singleThread := max(a.Config.Threads/total, 1)
	g, ctx := errgroup.WithContext(context.Background())

	for i := range a.Samples {
		sample := a.Samples[i]
		sampleStats := a.MutationStats.GetOrCreateSampleStats(sample)

		g.Go(func() error {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return ctx.Err()
			}
			err := alignSample(sample, singleThread, a.Config.Alignment.MapQThreshold)
			if err != nil {
				return err
			}
			err = sampleStats.ProcessBAMFile(ctx, a.Config.NMerSize, a.Config.MaxSubstitutions)
			if err != nil {
				return err
			}
			a.MutationStats.UpdateStatsFromSampleStats(sampleStats)
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return err
	}

	for _, result := range a.Samples {
		if result.Error != nil {
			failed++
			continue
		}

		successful++

		// 更新样品信息

		slog.Debug("比对完成",
			"样本", result.Name,
			"映射读数", result.AlignmentResult.MappedReads,
			"映射率", result.AlignmentResult.MappingRate)
	}

	if successful < total {
		return fmt.Errorf("比对失败，有 %d 个样本未成功比对", total-successful)
	}

	return nil
}

// 单个样品的比对
func alignSample(sample *cfg.Sample, threads, mapQThreshold int) error {
	// 检查输入文件是否存在
	inputFile := filepath.Join(sample.OutputDir, "target_only_reads.fastq.gz")
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("样本 %s 的输入文件不存在", sample.Name)
	}

	// 检查参考序列文件
	if sample.ReferenceFile == "" {
		return fmt.Errorf("样本 %s 没有参考序列文件", sample.Name)
	}

	// 调用通用的比对方法
	bamFile, err := AlignSampleWithParams(
		inputFile,
		sample.ReferenceFile,
		filepath.Join(sample.OutputDir, sample.Name),
		threads,
	)
	sample.BamFile = bamFile
	if err != nil {
		sample.Error = fmt.Errorf("比对失败: %w", err)
		return sample.Error
	}

	// 分析比对结果
	alignment, err := analyzeBamFile(bamFile, sample, mapQThreshold)
	sample.AlignmentResult = alignment
	if err != nil {
		sample.Error = fmt.Errorf("分析BAM失败: %w", err)
		return sample.Error
	}

	return nil
}

func AlignSampleWithParams(inputFile, referenceFile, outputPrefix string, threads int) (string, error) {
	var (
		samFile  = outputPrefix + ".sam"
		bamFile  = outputPrefix + ".sorted.bam"
		doneFile = bamFile + ".done"
	)

	// 检查是否已完成比对
	if _, err := os.Stat(doneFile); err == nil {
		// 检查BAM文件是否存在
		if _, err := os.Stat(bamFile); err == nil {
			slog.Debug("比对已完成，跳过", "prefix", outputPrefix)
			return bamFile, nil
		}
	}

	// 检查输入文件是否存在
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return "", fmt.Errorf("输入文件不存在: %s", inputFile)
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
		inputFile,
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
