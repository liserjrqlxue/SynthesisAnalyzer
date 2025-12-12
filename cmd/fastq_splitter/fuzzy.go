package main

import "strings"

// 增强的提取函数，处理更多边界情况
func (s *RegexpSplitter) extractTargetRegionEnhanced(sequence string) (string, *SampleInfo, string) {
	// 尝试多种匹配策略

	// 策略1：精确正则匹配
	targetRegion, sample, direction := s.extractTargetRegion(sequence)
	if sample != nil {
		return targetRegion, sample, direction
	}

	// 方法2：如果正则失败，尝试模糊匹配（如果启用）
	if s.config.AllowMismatch > 0 {
		targetRegion, sample, direction = s.fuzzyExtractTargetRegion(sequence)
		if sample != nil {
			return targetRegion, sample, direction
		}

		// 方法3：尝试基于位置的提取
		targetRegion, sample, direction = s.positionBasedExtract(sequence)
		if sample != nil {
			return targetRegion, sample, direction
		}
	}

	return "", nil, ""
}

// 模糊提取（允许错配）
func (s *RegexpSplitter) fuzzyExtractTargetRegion(sequence string) (string, *SampleInfo, string) {
	bestMatch := ""
	var bestSample *SampleInfo
	bestDirection := ""
	bestScore := 0

	seqLen := len(sequence)

	for _, sample := range s.samples {
		// 正向搜索
		headPos := s.fuzzySearch(sequence, sample.TargetSeq, s.config.AllowMismatch)
		if headPos != -1 {
			// 从头靶标位置向后搜索尾靶标
			searchStart := headPos + len(sample.TargetSeq)
			if searchStart < seqLen {
				tailPos := s.fuzzySearch(sequence[searchStart:], sample.PostTargetSeq, s.config.AllowMismatch)
				if tailPos != -1 {
					tailPos += searchStart
					// 提取区域
					region := sequence[headPos : tailPos+len(sample.PostTargetSeq)]
					// 计算匹配分数
					score := s.calculateMatchScore(region, sample.TargetSeq, sample.PostTargetSeq)
					if score > bestScore {
						bestScore = score
						bestMatch = region
						bestSample = sample
						bestDirection = "forward"
					}
				}
			}
		}

		// 反向搜索（如果需要）
		if s.useRC {
			rcHeadPos := s.fuzzySearch(sequence, sample.ReversePostTarget, s.config.AllowMismatch)
			if rcHeadPos != -1 {
				searchStart := rcHeadPos + len(sample.ReversePostTarget)
				if searchStart < seqLen {
					rcTailPos := s.fuzzySearch(sequence[searchStart:], sample.ReverseTarget, s.config.AllowMismatch)
					if rcTailPos != -1 {
						rcTailPos += searchStart
						// 提取并反向互补
						region := reverseComplement(sequence[rcHeadPos : rcTailPos+len(sample.ReverseTarget)])
						score := s.calculateMatchScore(region, sample.TargetSeq, sample.PostTargetSeq)
						if score > bestScore {
							bestScore = score
							bestMatch = region
							bestSample = sample
							bestDirection = "reverse"
						}
					}
				}
			}
		}
	}

	// 如果分数达到阈值，返回结果
	if bestScore >= s.config.MatchThreshold {
		return bestMatch, bestSample, bestDirection
	}

	return "", nil, ""
}

// 模糊搜索（允许错配）
func (s *RegexpSplitter) fuzzySearch(text, pattern string, maxMismatch int) int {
	patternLen := len(pattern)
	textLen := len(text)

	for i := 0; i <= textLen-patternLen; i++ {
		mismatch := 0
		for j := 0; j < patternLen; j++ {
			if text[i+j] != pattern[j] {
				mismatch++
				if mismatch > maxMismatch {
					break
				}
			}
		}
		if mismatch <= maxMismatch {
			return i
		}
	}

	return -1
}

// 计算匹配分数
func (s *RegexpSplitter) calculateMatchScore(region, headSeq, tailSeq string) int {
	score := 0

	// 检查头尾靶标是否完整
	if strings.HasPrefix(region, headSeq) {
		score += len(headSeq) * 2
	} else {
		// 部分匹配
		prefixLen := 0
		for i := 0; i < min(len(headSeq), len(region)); i++ {
			if region[i] == headSeq[i] {
				prefixLen++
			}
		}
		score += prefixLen
	}

	if strings.HasSuffix(region, tailSeq) {
		score += len(tailSeq) * 2
	} else {
		// 部分匹配
		suffixLen := 0
		regionLen := len(region)
		tailLen := len(tailSeq)

		for i := 0; i < min(tailLen, regionLen); i++ {
			if region[regionLen-1-i] == tailSeq[tailLen-1-i] {
				suffixLen++
			}
		}
		score += suffixLen
	}

	return score
}

// 定义 positionBasedExtract 函数
func (s *RegexpSplitter) positionBasedExtract(sequence string) (string, *SampleInfo, string) {
	// 基于位置的提取方法，不依赖正则表达式
	// 直接在序列中搜索头尾靶标，然后提取中间部分

	seqLen := len(sequence)
	if seqLen < 50 { // 序列太短
		return "", nil, ""
	}

	bestMatch := ""
	var bestSample *SampleInfo
	bestDirection := ""
	bestScore := 0

	for _, sample := range s.samples {
		// 正向搜索
		headPos := strings.Index(sequence, sample.TargetSeq)
		if headPos != -1 {
			// 从头靶标之后开始搜索尾靶标
			searchStart := headPos + len(sample.TargetSeq)
			if searchStart < seqLen {
				// 在剩余序列中搜索尾靶标
				tailPos := strings.Index(sequence[searchStart:], sample.PostTargetSeq)
				if tailPos != -1 {
					tailPos += searchStart
					// 提取从头靶标开始到尾靶标结束的区域
					region := sequence[headPos : tailPos+len(sample.PostTargetSeq)]

					// 计算匹配分数（完全匹配得高分）
					score := len(region) * 2
					if score > bestScore {
						bestScore = score
						bestMatch = region
						bestSample = sample
						bestDirection = "forward"
					}
				}
			}
		}

		// 反向搜索
		if s.useRC {
			// 搜索反向互补靶标
			rcHeadPos := strings.Index(sequence, sample.ReversePostTarget)
			if rcHeadPos != -1 {
				searchStart := rcHeadPos + len(sample.ReversePostTarget)
				if searchStart < seqLen {
					rcTailPos := strings.Index(sequence[searchStart:], sample.ReverseTarget)
					if rcTailPos != -1 {
						rcTailPos += searchStart
						// 提取区域并反向互补
						region := reverseComplement(sequence[rcHeadPos : rcTailPos+len(sample.ReverseTarget)])
						score := len(region) * 2
						if score > bestScore {
							bestScore = score
							bestMatch = region
							bestSample = sample
							bestDirection = "reverse"
						}
					}
				}
			}
		}
	}

	// 设置阈值，确保匹配质量
	if bestScore >= 40 { // 至少20bp的有效区域
		return bestMatch, bestSample, bestDirection
	}

	return "", nil, ""
}
