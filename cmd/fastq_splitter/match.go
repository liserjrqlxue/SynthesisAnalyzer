package main

import (
	"fmt"

	"github.com/bits-and-blooms/bloom/v3"
)

// 构建完全匹配索引
func (s *ExactMatchSplitter) buildIndex() error {
	fmt.Println("步骤5: 构建完全匹配索引（支持反向互补）...")

	// 创建Bloom过滤器
	expectedElements := uint(len(s.samples) * 4) // 正向+反向各2个序列
	s.bloomFilter = bloom.NewWithEstimates(
		expectedElements, // 估计元素数量
		0.001,            // 误报率
	)

	// 为每个样品提取barcode
	for _, sample := range s.samples {
		// 构建完整参考序列（用于后续分析）
		sample.FullReference = sample.TargetSeq + sample.SynthesisSeq + sample.PostTargetSeq

		// 计算反向互补序列
		sample.ReverseTarget = reverseComplement(sample.TargetSeq)
		sample.ReversePostTarget = reverseComplement(sample.PostTargetSeq)

		// 构建正向索引（头靶标+尾靶标）
		forwardKey := fmt.Sprintf("%s|%s", sample.TargetSeq, sample.PostTargetSeq)
		s.forwardIndex[forwardKey] = sample

		// 构建反向索引（反向头+反向尾）
		reverseKey := fmt.Sprintf("%s|%s", sample.ReverseTarget, sample.ReversePostTarget)
		s.reverseIndex[reverseKey] = sample

		// 添加到Bloom过滤器
		s.bloomFilter.Add([]byte(forwardKey))
		s.bloomFilter.Add([]byte(reverseKey))

		// 同时添加子序列到过滤器（提高过滤效率）
		s.bloomFilter.Add([]byte(sample.TargetSeq))
		s.bloomFilter.Add([]byte(sample.PostTargetSeq))
		s.bloomFilter.Add([]byte(sample.ReverseTarget))
		s.bloomFilter.Add([]byte(sample.ReversePostTarget))

		fmt.Printf("  样本 %s:\n", sample.Name)
		fmt.Printf("    正向: 头[%s] 尾[%s]\n",
			sample.TargetSeq, sample.PostTargetSeq)
		fmt.Printf("    反向: 头[%s] 尾[%s]\n",
			sample.ReverseTarget, sample.ReversePostTarget)
	}

	fmt.Printf("索引构建完成: %d 个样本\n", len(s.samples))
	return nil
}

// 在序列中搜索头靶标（从头部开始向后搜索）
func findHeadBarcode(sequence, headBarcode string, searchWindow int) (int, bool) {
	if searchWindow <= 0 || searchWindow > len(sequence) {
		searchWindow = len(sequence)
	}

	// 从序列开头向后搜索
	maxStart := min(len(sequence)-len(headBarcode), searchWindow)

	for start := 0; start <= maxStart; start++ {
		end := start + len(headBarcode)
		if end > len(sequence) {
			break
		}

		// 完全匹配检查
		if sequence[start:end] == headBarcode {
			return start, true
		}
	}

	return -1, false
}

// 在序列中搜索尾靶标（从尾部开始向前搜索）
func findTailBarcode(sequence, tailBarcode string, searchWindow int) (int, bool) {
	if searchWindow <= 0 || searchWindow > len(sequence) {
		searchWindow = len(sequence)
	}

	// 从序列尾部向前搜索
	minStart := max(0, len(sequence)-searchWindow)

	for start := len(sequence) - len(tailBarcode); start >= minStart; start-- {
		if start < 0 {
			break
		}

		end := start + len(tailBarcode)
		if end > len(sequence) {
			continue
		}

		// 完全匹配检查
		if sequence[start:end] == tailBarcode {
			return start, true
		}
	}

	return -1, false
}

// 完全匹配函数（支持正向和反向）
func (s *ExactMatchSplitter) exactMatch(sequence string) (*SampleInfo, string) {
	seqLen := len(sequence)
	if seqLen < 20 { // 序列太短
		return nil, ""
	}

	// 尝试正向匹配
	if sample, direction := s.matchForward(sequence); sample != nil {
		return sample, direction
	}

	// 尝试反向匹配（检查反向互补序列）
	if sample, direction := s.matchReverse(sequence); sample != nil {
		return sample, direction
	}

	return nil, ""
}

// 正向匹配
func (s *ExactMatchSplitter) matchForward(sequence string) (*SampleInfo, string) {
	// 快速过滤：检查是否有可能匹配
	if !s.mightContainAnyBarcode(sequence) {
		return nil, ""
	}

	var matchedSamples []*SampleInfo

	// 检查每个样本
	for _, sample := range s.samples {
		// 搜索头靶标（从头部向后）
		headPos, headFound := findHeadBarcode(sequence, sample.TargetSeq, s.config.SearchWindow)
		if !headFound {
			continue
		}

		// 搜索尾靶标（从尾部向前）
		tailPos, tailFound := findTailBarcode(sequence, sample.PostTargetSeq, s.config.SearchWindow)
		if !tailFound {
			continue
		}

		// 确保头在尾之前
		if headPos < tailPos {
			matchedSamples = append(matchedSamples, sample)
		}
	}

	// 检查匹配结果
	if len(matchedSamples) == 1 {
		return matchedSamples[0], "forward"
	} else if len(matchedSamples) > 1 {
		s.statsMutex.Lock()
		s.stats.ambiguousReads++
		s.statsMutex.Unlock()
	}

	return nil, ""
}

// 反向匹配
func (s *ExactMatchSplitter) matchReverse(sequence string) (*SampleInfo, string) {
	// 计算反向互补序列
	rcSequence := reverseComplement(sequence)

	var matchedSamples []*SampleInfo

	// 检查每个样本的反向序列
	for _, sample := range s.samples {
		// 搜索反向头靶标（在反向序列中从头部向后）
		headPos, headFound := findHeadBarcode(rcSequence, sample.ReverseTarget, s.config.SearchWindow)
		if !headFound {
			continue
		}

		// 搜索反向尾靶标（在反向序列中从尾部向前）
		tailPos, tailFound := findTailBarcode(rcSequence, sample.ReversePostTarget, s.config.SearchWindow)
		if !tailFound {
			continue
		}

		// 确保头在尾之前
		if headPos < tailPos {
			matchedSamples = append(matchedSamples, sample)
		}
	}

	// 检查匹配结果
	if len(matchedSamples) == 1 {
		return matchedSamples[0], "reverse"
	} else if len(matchedSamples) > 1 {
		s.statsMutex.Lock()
		s.stats.ambiguousReads++
		s.statsMutex.Unlock()
	}

	return nil, ""
}

// Bloom过滤器预检查
func (s *ExactMatchSplitter) mightContainAnyBarcode(sequence string) bool {
	// 提取序列的前后部分进行快速过滤
	headRegion := ""
	tailRegion := ""

	if len(sequence) > 50 {
		headRegion = sequence[:50]
		tailRegion = sequence[len(sequence)-50:]
	} else {
		headRegion = sequence
		tailRegion = sequence
	}

	// 检查是否可能包含任何已知的barcode
	return s.bloomFilter.Test([]byte(headRegion)) || s.bloomFilter.Test([]byte(tailRegion))
}
