package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/bits-and-blooms/bloom/v3"
)

// 构建完全匹配索引
func (s *RegexpSplitter) buildIndex() error {
	fmt.Println("构建正则表达式索引...")

	// 创建Bloom过滤器
	expectedElements := uint(len(s.samples) * 4) // 正向+反向各2个序列
	s.bloomFilter = bloom.NewWithEstimates(
		expectedElements, // 估计元素数量
		0.001,            // 误报率
	)

	// 为每个样品提取barcode
	for _, sample := range s.samples {
		// 1. 构建正向正则表达式
		// 模式: 头靶标 + (.*?) + 尾靶标
		forwardPattern := s.escapeRegexp(sample.TargetSeq) + `(.*?)` + s.escapeRegexp(sample.PostTargetSeq)
		forwardRegex, err := regexp.Compile(forwardPattern)
		if err != nil {
			return fmt.Errorf("构建正向正则表达式失败（样本%s）: %v", sample.Name, err)
		}

		// 2. 如果需要，构建反向互补正则表达式
		var reverseRegex *regexp.Regexp
		var reversePattern string
		if s.useRC {
			// 计算反向互补序列
			rcTarget := reverseComplement(sample.TargetSeq)
			rcPostTarget := reverseComplement(sample.PostTargetSeq)

			reversePattern := s.escapeRegexp(rcTarget) + `(.*?)` + s.escapeRegexp(rcPostTarget)
			reverseRegex, err = regexp.Compile(reversePattern)
			if err != nil {
				return fmt.Errorf("构建反向正则表达式失败（样本%s）: %v", sample.Name, err)
			}
		}
		s.seqRegexps = append(s.seqRegexps, &SeqRegexp{
			sample:       sample,
			forwardRegex: forwardRegex,
			reverseRegex: reverseRegex,
		})

		fmt.Printf("  样本 %s: 正则表达式构建成功\n", sample.Name)
		fmt.Printf("    正向: %s\n", forwardPattern)
		if s.useRC && reverseRegex != nil {
			fmt.Printf("    反向: %s\n", reversePattern)
		}

		fmt.Printf("正则表达式索引构建完成: %d 个样本\n", len(s.samples))

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

// 转义正则表达式特殊字符
func (s *RegexpSplitter) escapeRegexp(seq string) string {
	// 正则表达式特殊字符: . * + ? ^ $ { } [ ] ( ) | \
	specialChars := []string{".", "*", "+", "?", "^", "$", "{", "}", "[", "]", "(", ")", "|", "\\"}

	escaped := seq
	for _, char := range specialChars {
		escaped = strings.ReplaceAll(escaped, char, "\\"+char)
	}

	return escaped
}

// 使用正则表达式匹配序列
func (s *RegexpSplitter) matchWithRegexp(sequence string) (*SampleInfo, string, []string) {
	// 首先尝试正向匹配
	for _, seqRegexp := range s.seqRegexps {
		if matches := seqRegexp.forwardRegex.FindStringSubmatch(sequence); matches != nil {
			// 检查匹配位置：确保头靶标在前，尾靶标在后
			headPos := strings.Index(sequence, seqRegexp.sample.TargetSeq)
			tailPos := strings.Index(sequence, seqRegexp.sample.PostTargetSeq)

			if headPos != -1 && tailPos != -1 && headPos < tailPos {
				return seqRegexp.sample, "forward", matches
			}
		}
	}

	// 如果正向未匹配且允许反向互补，尝试反向匹配
	if s.useRC {
		for _, seqRegexp := range s.seqRegexps {
			if seqRegexp.reverseRegex == nil {
				continue
			}

			if matches := seqRegexp.reverseRegex.FindStringSubmatch(sequence); matches != nil {
				// 检查匹配位置
				// headPos := strings.Index(sequence, seqRegexp.sample.ReverseTarget)
				// tailPos := strings.Index(sequence, seqRegexp.sample.ReversePostTarget)

				// if headPos != -1 && tailPos != -1 && headPos < tailPos {
				return seqRegexp.sample, "reverse", matches
				// }
			}
		}
	}

	return nil, "", nil
}

// 处理单个FASTQ记录
func (s *RegexpSplitter) processRecordWithRegexp(record []byte) (*SampleInfo, string, []byte) {
	// 解析FASTQ记录，提取序列
	lines := bytes.SplitN(record, []byte{'\n'}, 4)
	if len(lines) < 2 {
		return nil, "", nil
	}

	sequence := string(lines[1])

	// 使用正则表达式匹配
	sample, direction, matches := s.matchWithRegexp(sequence)
	if sample == nil {
		return nil, "", nil
	}

	// 更新样本统计
	sample.TotalReads++
	sample.MatchedReads++
	if direction == "forward" {
		sample.ForwardReads++
	} else {
		sample.ReverseReads++
	}

	// 如果是反向匹配，需要对序列进行反向互补
	var processedRecord []byte
	if direction == "reverse" {
		processedRecord = s.reverseComplementRecord(record)
	} else {
		processedRecord = record
	}

	// 可选：提取中间序列用于后续分析
	if matches != nil && len(matches) > 1 {
		// matches[1] 是 (.*?) 匹配到的中间序列
		// 这里可以保存中间序列用于分析
	}

	return sample, direction, processedRecord
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
func (s *RegexpSplitter) exactMatch(sequence string) (*SampleInfo, string) {
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
func (s *RegexpSplitter) matchForward(sequence string) (*SampleInfo, string) {
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
func (s *RegexpSplitter) matchReverse(sequence string) (*SampleInfo, string) {
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
func (s *RegexpSplitter) mightContainAnyBarcode(sequence string) bool {
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
