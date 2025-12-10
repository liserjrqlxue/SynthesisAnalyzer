package synthesis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

func (a *SynthesisAnalyzer) alignAndAnalyze(splitFiles []string) error {
	// 为每个参考序列创建索引
	var wg sync.WaitGroup
	sem := make(chan struct{}, a.config.Threads) // 控制并发数

	for idx, ref := range a.references {
		wg.Add(1)
		sem <- struct{}{}

		go func(refIdx int, refID, refSeq string) {
			defer wg.Done()
			defer func() { <-sem }()

			// 1. 为单个参考序列创建minimap2索引
			refFile := filepath.Join(a.config.OutputDir,
				fmt.Sprintf("ref_%s.fa", refID))
			a.createReferenceFile(refFile, refSeq)
			defer os.Remove(refFile)

			// 2. 使用minimap2比对（比BWA更快）
			samFile := filepath.Join(a.config.OutputDir,
				fmt.Sprintf("align_%s.sam", refID))
			a.runMinimap2(refFile, splitFiles[refIdx], samFile)

			// 3. 解析SAM文件并统计
			a.analyzeSAM(samFile, refIdx)

			// 4. 清理临时文件
			os.Remove(samFile)

		}(idx, ref.ID, ref.Sequence)
	}

	wg.Wait()
	return nil
}

func (a *SynthesisAnalyzer) runMinimap2(refFile, readsFile, samFile string) error {
	// minimap2对于短参考序列比对更快
	cmd := exec.Command("minimap2",
		"-a",            // 输出SAM格式
		"-x", "map-ont", // 针对合成序列的预设
		"-t", "2", // 每个比对使用2线程
		"--secondary=no", // 不输出secondary比对
		"-o", samFile,
		refFile,
		readsFile,
	)

	// 设置内存限制
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("MINIMAP2_MAX_MEM=%dM", a.config.MemoryGB*1024/len(a.references)),
	)

	return cmd.Run()
}
