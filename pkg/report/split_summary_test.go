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

生成时间: 2026-03-20 10:08:10
程序版本: v1.0
测序时间: 2026.03.19

运行配置
--------
Excel文件: input-260303C-FT100120710.xlsx
输出目录: input-260303C-FT100120710
线程数: 128
启用反向互补: true
质量阈值: 20
最小合并长度: 80
跳过已存在文件: true
压缩输出: true

总体统计
--------
合并文件数: 1
样品总数: 48
总处理reads数: 36315768
总处理bases数: 5447365200
过滤拼接后reads数: 17981735
过滤拼接后bases数: 3654802344
成功匹配reads数: 147 (0.0%)
匹配失败reads数: 17981588 (100.0%)
`

	summaryPath := filepath.Join(tempDir, "split_summary.txt")
	err = os.WriteFile(summaryPath, []byte(summaryContent), 0644)
	if err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 调用 ReadSplitSummary 函数
	totalReads, sequencingDate, filteredReads, matchedReads, err := ReadSplitSummary(tempDir)
	if err != nil {
		t.Errorf("ReadSplitSummary 失败: %v", err)
		return
	}

	// 验证结果
	expectedTotalReads := 36315768
	expectedSequencingDate := "2026.03.19"
	expectedFilteredReads := 17981735
	expectedMatchedReads := 147

	if totalReads != expectedTotalReads {
		t.Errorf("TotalReads 错误: 期望 %d, 实际 %d", expectedTotalReads, totalReads)
	}

	if sequencingDate != expectedSequencingDate {
		t.Errorf("SequencingDate 错误: 期望 %s, 实际 %s", expectedSequencingDate, sequencingDate)
	}

	if filteredReads != expectedFilteredReads {
		t.Errorf("FilteredReads 错误: 期望 %d, 实际 %d", expectedFilteredReads, filteredReads)
	}

	if matchedReads != expectedMatchedReads {
		t.Errorf("MatchedReads 错误: 期望 %d, 实际 %d", expectedMatchedReads, matchedReads)
	}

	// 打印测试结果
	t.Logf("测试通过! TotalReads: %d, SequencingDate: %s, FilteredReads: %d, MatchedReads: %d", totalReads, sequencingDate, filteredReads, matchedReads)
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
过滤拼接后reads数: 5286911
成功匹配reads数: 8533735 (80.7%)
匹配失败reads数: 2040088 (19.3%)
`

	summaryPath := filepath.Join(tempDir, "split_summary.txt")
	err = os.WriteFile(summaryPath, []byte(summaryContent), 0644)
	if err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 调用 ReadSplitSummary 函数
	totalReads, sequencingDate, filteredReads, matchedReads, err := ReadSplitSummary(tempDir)
	if err != nil {
		t.Errorf("ReadSplitSummary 失败: %v", err)
		return
	}

	// 验证结果
	expectedTotalReads := 10573823
	expectedSequencingDate := ""
	expectedFilteredReads := 5286911
	expectedMatchedReads := 8533735

	if totalReads != expectedTotalReads {
		t.Errorf("TotalReads 错误: 期望 %d, 实际 %d", expectedTotalReads, totalReads)
	}

	if sequencingDate != expectedSequencingDate {
		t.Errorf("SequencingDate 错误: 期望 %s, 实际 %s", expectedSequencingDate, sequencingDate)
	}

	if filteredReads != expectedFilteredReads {
		t.Errorf("FilteredReads 错误: 期望 %d, 实际 %d", expectedFilteredReads, filteredReads)
	}

	if matchedReads != expectedMatchedReads {
		t.Errorf("MatchedReads 错误: 期望 %d, 实际 %d", expectedMatchedReads, matchedReads)
	}

	// 打印测试结果
	t.Logf("测试通过! TotalReads: %d, SequencingDate: %s, FilteredReads: %d, MatchedReads: %d", totalReads, sequencingDate, filteredReads, matchedReads)
}
