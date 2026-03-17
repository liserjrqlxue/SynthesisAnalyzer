package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSplitSummary(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建测试用的 split_summary.txt 文件
	summaryContent := `FASTQ拆分汇总报告
=====================

生成时间: 2026-03-17 17:31:28
程序版本: v1.0
测序时间: 2026.03.13

运行配置
--------
Excel文件: input-260305B-FT100120704.xlsx
输出目录: input-260305B-FT100120704
线程数: 128
启用反向互补: true
质量阈值: 20
最小合并长度: 80
跳过已存在文件: true
压缩输出: true

总体统计
--------
合并文件数: 1
样品总数: 87
总处理reads数: 10573823
成功匹配reads数: 8533735 (80.7%)
匹配失败reads数: 2040088 (19.3%)
`

	summaryPath := filepath.Join(tempDir, "split_summary.txt")
	err = os.WriteFile(summaryPath, []byte(summaryContent), 0644)
	if err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 调用 ReadSplitSummary 函数
	barcodeReads, sequencingDate, err := ReadSplitSummary(tempDir)
	if err != nil {
		t.Errorf("ReadSplitSummary 失败: %v", err)
		return
	}

	// 验证结果
	expectedBarcodeReads := 10573823
	expectedSequencingDate := "2026.03.13"

	if barcodeReads != expectedBarcodeReads {
		t.Errorf("BarcodeReads 错误: 期望 %d, 实际 %d", expectedBarcodeReads, barcodeReads)
	}

	if sequencingDate != expectedSequencingDate {
		t.Errorf("SequencingDate 错误: 期望 %s, 实际 %s", expectedSequencingDate, sequencingDate)
	}

	// 打印测试结果
	t.Logf("测试通过! BarcodeReads: %d, SequencingDate: %s", barcodeReads, sequencingDate)
}

func TestReadSplitSummaryWithoutSequencingDate(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建测试用的 split_summary.txt 文件（不含测序时间）
	summaryContent := `FASTQ拆分汇总报告
=====================

生成时间: 2026-03-17 17:31:28
程序版本: v1.0

运行配置
--------
Excel文件: input-260305B-FT100120704.xlsx
输出目录: input-260305B-FT100120704
线程数: 128
启用反向互补: true
质量阈值: 20
最小合并长度: 80
跳过已存在文件: true
压缩输出: true

总体统计
--------
合并文件数: 1
样品总数: 87
总处理reads数: 10573823
成功匹配reads数: 8533735 (80.7%)
匹配失败reads数: 2040088 (19.3%)
`

	summaryPath := filepath.Join(tempDir, "split_summary.txt")
	err = os.WriteFile(summaryPath, []byte(summaryContent), 0644)
	if err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 调用 ReadSplitSummary 函数
	barcodeReads, sequencingDate, err := ReadSplitSummary(tempDir)
	if err != nil {
		t.Errorf("ReadSplitSummary 失败: %v", err)
		return
	}

	// 验证结果
	expectedBarcodeReads := 10573823
	expectedSequencingDate := ""

	if barcodeReads != expectedBarcodeReads {
		t.Errorf("BarcodeReads 错误: 期望 %d, 实际 %d", expectedBarcodeReads, barcodeReads)
	}

	if sequencingDate != expectedSequencingDate {
		t.Errorf("SequencingDate 错误: 期望 %s, 实际 %s", expectedSequencingDate, sequencingDate)
	}

	// 打印测试结果
	t.Logf("测试通过! BarcodeReads: %d, SequencingDate: %s", barcodeReads, sequencingDate)
}
