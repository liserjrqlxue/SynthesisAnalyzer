package synthesis

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func (a *SynthesisAnalyzer) mergeWithFastp() (string, error) {
	outputFile := filepath.Join(a.config.OutputDir, "merged.fastq")

	// 构建fastp命令
	cmd := exec.Command("fastp",
		"-i", a.config.InputR1,
		"-I", a.config.InputR2,
		"--merged_out", outputFile,
		"--out1", "/dev/null",
		"--out2", "/dev/null",
		"--unpaired1", "/dev/null",
		"--unpaired2", "/dev/null",
		"--thread", strconv.Itoa(a.config.Threads/2), // 分配部分线程
		"--merge",
		"--merge_len", strconv.Itoa(a.config.MergeLen),
		"--qualified_quality_phred", strconv.Itoa(a.config.QualityCutoff),
		"--correction",
		"--overlap_len_require", "30",
		"--overlap_diff_limit", "5",
		"--html", filepath.Join(a.config.OutputDir, "fastp_report.html"),
		"--json", filepath.Join(a.config.OutputDir, "fastp_report.json"),
	)

	// 设置资源限制
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// 执行并监控进度
	progress := make(chan float64)
	go a.monitorFastpProgress(cmd, progress)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("fastp执行失败: %v", err)
	}

	return outputFile, nil
}
