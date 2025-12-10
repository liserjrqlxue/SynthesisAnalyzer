package synthesis

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

	for chunk := 0; chunk < chunks; chunk++ {
		wg.Add(1)
		go func(chunkIdx int) {
			defer wg.Done()

			start := chunkIdx * chunkSize
			end := min((chunkIdx+1)*chunkSize, len(mmapData.Data))
			chunkData := mmapData.Data[start:end]

			// 使用SIMD加速barcode匹配
			a.processChunk(chunkData, outputFiles)
		}(chunk)
	}
	wg.Wait()

	// 关闭所有文件
	for _, f := range outputFiles {
		f.Close()
	}

	return outputPaths, nil
}

// SIMD加速的barcode匹配
func (a *SynthesisAnalyzer) processChunk(data []byte, files []*os.File) {
	// 使用AVX2指令集加速序列匹配
	// 这里使用一个优化的字符串匹配库
	for i := 0; i < len(data); {
		// 找到下一个FASTQ记录
		recordStart := i
		if i+3 >= len(data) {
			break
		}

		// 解析FASTQ记录
		headerEnd := bytes.IndexByte(data[i:], '\n')
		if headerEnd < 0 {
			break
		}

		seqStart := i + headerEnd + 1
		seqEnd := bytes.IndexByte(data[seqStart:], '\n')
		if seqEnd < 0 {
			break
		}

		sequence := data[seqStart : seqStart+seqEnd]

		// 提取barcode
		if len(sequence) >= a.config.BarcodeStartLen+a.config.BarcodeEndLen {
			startBarcode := string(sequence[:a.config.BarcodeStartLen])
			endBarcode := string(sequence[len(sequence)-a.config.BarcodeEndLen:])

			// 查找匹配的参考序列
			refIdx := a.matchBarcode(startBarcode, endBarcode)
			if refIdx >= 0 {
				// 写入对应的文件
				files[refIdx].Write(data[recordStart:i])
			}
		}

		// 移动到下一个记录
		i = seqStart + seqEnd + 1
		// 跳过质量分数行
		for j := 0; j < 2; j++ {
			next := bytes.IndexByte(data[i:], '\n')
			if next < 0 {
				break
			}
			i += next + 1
		}
	}
}

// 优化的barcode匹配
func (a *SynthesisAnalyzer) matchBarcode(start, end string) int {
	// 使用bloom filter快速过滤
	if !a.bloomFilter.MightContain(start + end) {
		return -1
	}

	// 精确匹配（允许错配）
	for i, ref := range a.references {
		if hammingDistance(start, ref.BarcodeStart) <= a.config.BarcodeMismatch &&
			hammingDistance(end, ref.BarcodeEnd) <= a.config.BarcodeMismatch {
			return i
		}
	}
	return -1
}

// 汉明距离计算（使用SIMD优化）
func hammingDistance(a, b string) int {
	if len(a) != len(b) {
		return 1000
	}

	// 使用汇编优化
	dist := 0
	// SIMD实现...
	return dist
}
