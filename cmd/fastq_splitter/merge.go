package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 合并PE reads
func (s *RegexpSplitter) mergeReads() error {
	// 去重R1/R2文件对
	filePairs := make(map[string][]*SampleInfo) // key: "R1|R2" -> 样品列表

	for _, sample := range s.samples {
		key := sample.R1Path + "|" + sample.R2Path
		filePairs[key] = append(filePairs[key], sample)
	}

	fmt.Printf("  需要合并 %d 个唯一的R1/R2文件对\n", len(filePairs))

	// 创建合并目录
	mergedDir := filepath.Join(s.config.OutputDir, "merged")
	if err := os.MkdirAll(mergedDir, 0755); err != nil {
		return fmt.Errorf("创建合并目录失败: %v", err)
	}

	// 并行合并
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.config.Threads/2) // 限制并发数
	results := make(chan struct {
		key    string
		output string
		err    error
	}, len(filePairs))

	var mergedCount int64
	var skippedCount int64

	for key, samples := range filePairs {
		wg.Add(1)
		sem <- struct{}{}

		go func(k string, samps []*SampleInfo) {
			defer func() {
				<-sem
				wg.Done()
			}()

			// 生成合并后的文件名
			mergedFile, exists := s.getMergedFileName(samps[0])

			result := struct {
				key    string
				output string
				err    error
			}{
				key:    k,
				output: mergedFile,
				err:    nil,
			}

			if exists && s.config.SkipExisting {
				atomic.AddInt64(&skippedCount, 1)
				results <- result
				return
			}

			// 运行fastp
			output, err := s.runFastpWithCheck(samps[0].R1Path, samps[0].R2Path, mergedFile)
			result.output = output
			result.err = err
			if err == nil {
				atomic.AddInt64(&mergedCount, 1)
				// 创建完成标签
				if err := s.createDoneFile(output); err != nil {
					log.Printf("警告: 创建完成标签失败: %v", err)
				}

				// 更新样品信息
				for _, sample := range samps {
					sample.MergeTime = time.Now()
				}
			}

			results <- result
		}(key, samples)
	}

	// 等待所有合并完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 处理结果
	errorCount := 0
	for result := range results {
		if result.err != nil {
			errorCount++
			log.Printf("合并失败 (R1: %s): %v", result.key, result.err)
			continue
		}

		// 更新对应样品的MergedFile字段
		samples := filePairs[result.key]
		for _, sample := range samples {
			sample.Status = "merged"
			sample.MergedFile = result.output
			s.fileMap[result.output] = append(s.fileMap[result.output], sample)
		}

		fmt.Printf("  合并完成: %s -> %s (%d个样品)\n",
			result.key, filepath.Base(result.output), len(samples))
	}

	fmt.Printf("  合并结果: %d 个已处理, %d 个已跳过, %d 个失败\n",
		mergedCount, skippedCount, errorCount)

	// 检查是否有样品合并失败
	for _, sample := range s.samples {
		if sample.MergedFile == "" {
			return fmt.Errorf("样品 %s 合并失败，请检查输入文件", sample.Name)
		}
	}

	return nil
}

// 生成合并文件名
func (s *RegexpSplitter) getMergedFileName(sample *SampleInfo) (string, bool) {
	// 基于R1和R2路径生成唯一的文件名
	base1 := filepath.Base(sample.R1Path)
	base2 := filepath.Base(sample.R2Path)

	// 去掉扩展名
	base1 = strings.TrimSuffix(base1, filepath.Ext(base1))
	base2 = strings.TrimSuffix(base2, filepath.Ext(base2))

	var base []byte
	for i := 0; i < min(len(base1), len(base2)); i++ {
		if base1[i] == base2[i] {
			base = append(base, base1[i])
		} else {
			break
		}
	}

	// 合并目录
	mergedDir := filepath.Join(s.config.OutputDir, "merged")
	os.MkdirAll(mergedDir, 0755)

	// 使用第一个样品的名称作为文件名
	filename := fmt.Sprintf("%s_merged.fastq.gz", sample.Name)
	if len(base) > 0 {
		filename = fmt.Sprintf("%smerged.fastq.gz", base)
	}
	mergedFile := filepath.Join(mergedDir, filename)
	doneFile := mergedFile + ".done"

	// 检查合并文件和完成标签是否都存在
	if _, err1 := os.Stat(mergedFile); err1 == nil {
		if _, err2 := os.Stat(doneFile); err2 == nil {
			// 检查文件大小是否合理（至少1KB）
			if fi, err := os.Stat(mergedFile); err == nil && fi.Size() > 1024 {
				fmt.Printf("    合并文件已存在且完成标签存在: %s\n", filepath.Base(mergedFile))
				return mergedFile, true
			}
		}
	}

	return mergedFile, false
}

// 创建完成标签
func (s *RegexpSplitter) createDoneFile(filename string) error {
	doneFile := filename + ".done"
	content := fmt.Sprintf("Created: %s\nFile: %s\n",
		time.Now().Format(time.RFC3339),
		filepath.Base(filename))

	return os.WriteFile(doneFile, []byte(content), 0644)
}
