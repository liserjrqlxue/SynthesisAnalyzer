package synthesis

import (
	"fmt"
)

func (a *SynthesisAnalyzer) Run() error {
	// 步骤2: 使用fastp合并PE reads
	// 步骤3: 按barcode拆分reads
	splitFiles, err := a.splitByBarcode(mergedFile)
	if err != nil {
		return fmt.Errorf("拆分reads失败: %v", err)
	}

	// 步骤5: 输出合成成功率报告
	if err := a.outputSynthesisReport(); err != nil {
		return fmt.Errorf("生成报告失败: %v", err)
	}

	return nil
}
