package synthesis

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/exp/mmap"
)

func (a *SynthesisAnalyzer) splitByBarcode(mergedFile string) ([]string, error) {
	// 创建输出文件
	outputFiles := make([]*os.File, len(a.references))
	outputPaths := make([]string, len(a.references))

	for i, ref := range a.references {
		path := filepath.Join(a.config.OutputDir,
			fmt.Sprintf("split_%s.fastq", ref.ID))
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		outputFiles[i] = f
		outputPaths[i] = path
	}

	// 使用mmap加速读取
	mmapData, err := mmap.Open(mergedFile)
	if err != nil {
		return nil, err
	}
	defer mmapData.Close()

	// 并发处理
	var wg sync.WaitGroup
	chunkSize := 64 * 1024 * 1024 // 64MB chunks
	chunks := len(mmapData.Data) / chunkSize

	// 关闭所有文件
	for _, f := range outputFiles {
		f.Close()
	}

	return outputPaths, nil
}
