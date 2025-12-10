package synthesis

import (
	"fmt"
	"os"
)

func (a *SynthesisAnalyzer) Run() error {
	// 步骤1: 加载参考序列并提取barcode
	if err := a.loadReferences(); err != nil {
		return fmt.Errorf("加载参考序列失败: %v", err)
	}

	// 步骤2: 使用fastp合并PE reads
	mergedFile, err := a.mergeWithFastp()
	if err != nil {
		return fmt.Errorf("合并reads失败: %v", err)
	}
	defer os.Remove(mergedFile)

	// 步骤3: 按barcode拆分reads
	splitFiles, err := a.splitByBarcode(mergedFile)
	if err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}
	defer a.cleanupTempFiles(splitFiles)

	// 步骤4: 并行比对并统计
	if err := a.alignAndAnalyze(splitFiles); err != nil {
		return fmt.Errorf("比对分析失败: %v", err)
	}

	// 步骤5: 输出合成成功率报告
	if err := a.outputSynthesisReport(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	return nil
}
