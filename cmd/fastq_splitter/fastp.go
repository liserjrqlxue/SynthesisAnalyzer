package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 带检查的fastp运行函数
func (s *EnhancedSplitter) runFastpWithCheck(r1Path, r2Path, outputFile string) (string, error) {
	// 确保输出文件以.gz结尾
	if !strings.HasSuffix(outputFile, ".gz") {
		outputFile = outputFile + ".gz"
	}

	fmt.Printf("    开始合并: %s + %s -> %s\n",
		filepath.Base(r1Path), filepath.Base(r2Path), filepath.Base(outputFile))

	startTime := time.Now()

	// 运行fastp
	output, err := s.runFastp(r1Path, r2Path, outputFile)

	if err != nil {
		return "", fmt.Errorf("fastp合并失败: %v", err)
	}

	elapsed := time.Since(startTime)
	fileSize := getFileSize(output)

	fmt.Printf("    合并完成: %s (%.2f MB, 耗时: %v)\n",
		filepath.Base(output), float64(fileSize)/1024/1024, elapsed)

	return output, nil
}

// 运行fastp进行合并
func (s *EnhancedSplitter) runFastp(r1Path, r2Path, outputFile string) (string, error) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("fastp_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %v", err)
	}

	defer func() {
		if !s.config.CleanupTemp {
			fmt.Printf("    临时文件保留在: %s\n", tempDir)
			return
		}
		// 清理临时目录
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("警告: 清理临时目录失败: %v", err)
		}
	}()

	if s.config.FastqDir != "" {
		r1Path = filepath.Join(s.config.FastqDir, r1Path)
		r2Path = filepath.Join(s.config.FastqDir, r2Path)
	}
	// 检查文件是否存在
	if _, err := os.Stat(r1Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R1文件不存在: %s", r1Path)
	}
	if _, err := os.Stat(r2Path); os.IsNotExist(err) {
		return "", fmt.Errorf("R2文件不存在: %s", r2Path)
	}

	// 构建fastp命令
	args := s.buildFastpCommand(r1Path, r2Path, outputFile, tempDir)
	cmd := exec.Command("fastp", args...)

	// 捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置超时（30分钟）
	timeout := 30 * time.Minute
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动fastp失败: %v", err)
	}

	log.Printf("CMD:\n%s", cmd)
	// 等待完成或超时
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return "", fmt.Errorf("fastp执行超时(%v)", timeout)
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("fastp执行失败: %v\n%s", err, stderr.String())
		}
	}

	// 检查输出
	if fi, err := os.Stat(outputFile); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("合并文件生成失败或为空")
	}

	// 读取合并统计
	stats, _ := s.readFastpStats(outputFile + ".fastp.json")
	fmt.Printf("    合并完成: %s,%s -> %s (%.2f MB)\n\t(统计: %v)\n",
		filepath.Base(r1Path),
		filepath.Base(r2Path),
		filepath.Base(outputFile),
		float64(getFileSize(outputFile))/1024/1024,
		stats,
	)

	return outputFile, nil
}

// 构建fastp命令
func (s *EnhancedSplitter) buildFastpCommand(r1Path, r2Path, outputFile, tempDir string) []string {
	args := []string{
		"-i", r1Path,
		"-I", r2Path,
		"--merged_out", outputFile,
		"--thread", "4",
		"--merge",
		"--overlap_len_require", strconv.Itoa(s.config.OverlapLenRequire),
		"--overlap_diff_limit", "5",
		"--qualified_quality_phred", fmt.Sprintf("%d", s.config.Quality),
		"--length_required", fmt.Sprintf("%d", s.config.MergeLen-50),
		"--correction",
		"--detect_adapter_for_pe",
		"--html", outputFile + ".fastp.html",
		"--json", outputFile + ".fastp.json",
		"",
	}

	// 添加压缩选项
	if strings.HasSuffix(outputFile, ".gz") {
		args = append(args, "--compression", fmt.Sprintf("%d", s.config.CompressLevel))
	} else {
		args = append(args, "--uncompressed")
	}

	// 设置临时输出文件
	out1File := filepath.Join(tempDir, "out1.fastq")
	out2File := filepath.Join(tempDir, "out2.fastq")
	args = append(args, "--out1", out1File, "--out2", out2File)

	return args
}

// 获取文件大小
func getFileSize(filename string) int64 {
	fi, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// 读取fastp统计信息
func (s *EnhancedSplitter) readFastpStats(jsonFile string) (map[string]interface{}, error) {
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	if summary, ok := result["summary"].(map[string]interface{}); ok {
		if before, ok := summary["before_filtering"].(map[string]interface{}); ok {
			stats["before_reads"] = before["total_reads"]
		}
		if after, ok := summary["after_filtering"].(map[string]interface{}); ok {
			stats["after_reads"] = after["total_reads"]
			stats["merged_reads"] = after["merged_reads"]
		}
	}

	return stats, nil
}
