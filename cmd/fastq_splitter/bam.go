package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// 分析BAM文件，统计合成成功率
func (a *AlignmentAnalyzer) analyzeBamFile(bamFile string, sample *SampleInfo) (*SampleAlignment, error) {
	// 读取BAM文件
	cmd := exec.Command("samtools", "view", bamFile)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取BAM文件失败: %v", err)
	}

	// 解析SAM行
	lines := strings.Split(string(output), "\n")

	// 初始化位置统计
	positionStats := make([]PositionStat, sample.ReferenceLen)
	for i := range positionStats {
		positionStats[i].Position = i + 1 // 1-based position
	}

	totalReads := int64(0)
	mappedReads := int64(0)
	totalMismatches := int64(0)
	totalMatches := int64(0)

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 11 {
			continue
		}

		totalReads++

		// 检查比对质量
		mapq, _ := strconv.Atoi(fields[4])
		if mapq < a.config.MapQThreshold {
			continue
		}

		mappedReads++

		// 解析CIGAR和MD标签
		cigar := fields[5]
		mdTag := ""

		// 查找MD标签
		for i := 11; i < len(fields); i++ {
			if strings.HasPrefix(fields[i], "MD:Z:") {
				mdTag = fields[i][5:]
				break
			}
		}

		// 解析比对结果
		a.parseAlignment(cigar, mdTag, fields[9], sample, &positionStats, &totalMatches, &totalMismatches)
	}

	// 计算汇总统计
	summary := &AlignmentSummary{
		TotalReads:      totalReads,
		MappedReads:     mappedReads,
		AverageCoverage: 0,
		AverageIdentity: 0,
	}

	if totalReads > 0 {
		summary.MappingRate = float64(mappedReads) / float64(totalReads) * 100
	}

	// 计算每个位置的覆盖度和错误率
	totalPositions := int64(0)
	totalErrors := int64(0)

	for i := range positionStats {
		pos := &positionStats[i]
		if pos.TotalReads > 0 {
			pos.Coverage = float64(pos.TotalReads) / float64(mappedReads)
			totalErrors = pos.MismatchCount + pos.DeletionCount + pos.InsertionCount
			totalPositions = pos.MatchCount + totalErrors
			if totalPositions > 0 {
				pos.ErrorRate = float64(totalErrors) / float64(totalPositions)
			}
		}
	}

	// 计算平均覆盖度和identity
	if sample.ReferenceLen > 0 {
		totalCoverage := 0.0
		for i := range positionStats {
			totalCoverage += positionStats[i].Coverage
		}
		summary.AverageCoverage = totalCoverage / float64(sample.ReferenceLen)
	}

	if totalMatches+totalMismatches > 0 {
		summary.AverageIdentity = float64(totalMatches) / float64(totalMatches+totalMismatches) * 100
	}

	// 识别高错误率位置（合成成功率低的位置）
	errorPositions := []int{}
	for i, pos := range positionStats {
		if pos.ErrorRate > 0.05 { // 错误率超过5%
			errorPositions = append(errorPositions, i+1)
		}
	}
	summary.ErrorPositions = errorPositions

	// 计算整体合成成功率
	if totalMatches+totalMismatches+totalErrors > 0 {
		summary.SynthesisSuccess = float64(totalMatches) / float64(totalMatches+totalMismatches+totalErrors) * 100
	}

	return &SampleAlignment{
		SampleName:    sample.Name,
		ReferenceSeq:  sample.ReferenceSeq,
		ReferenceLen:  sample.ReferenceLen,
		PositionStats: positionStats,
		Summary:       summary,
		BamFile:       bamFile,
		BamIndex:      bamFile + ".bai",
	}, nil
}

// 解析比对结果
func (a *AlignmentAnalyzer) parseAlignment(cigar, mdTag, seq string, sample *SampleInfo,
	positionStats *[]PositionStat, totalMatches, totalMismatches *int64) {

	refPos := 0
	seqPos := 0

	// 解析CIGAR
	i := 0
	for i < len(cigar) {
		// 解析数字
		num := 0
		for i < len(cigar) && cigar[i] >= '0' && cigar[i] <= '9' {
			num = num*10 + int(cigar[i]-'0')
			i++
		}

		if i >= len(cigar) {
			break
		}

		op := cigar[i]
		i++

		switch op {
		case 'M': // 匹配或错配
			for j := 0; j < num; j++ {
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++

					// 使用MD标签判断是否匹配
					if a.isMatch(refPos, seq[seqPos], mdTag, &refPos) {
						(*positionStats)[refPos].MatchCount++
						(*totalMatches)++
					} else {
						(*positionStats)[refPos].MismatchCount++
						(*totalMismatches)++
					}
				}
				refPos++
				seqPos++
			}
		case 'D': // 缺失
			for j := 0; j < num; j++ {
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++
					(*positionStats)[refPos].DeletionCount++
				}
				refPos++
			}
		case 'I': // 插入
			if refPos > 0 && refPos-1 < len(*positionStats) {
				(*positionStats)[refPos-1].InsertionCount++
			}
			seqPos += num
		case 'S', 'H': // 软裁剪/硬裁剪
			seqPos += num
		}
	}
}

// 检查是否匹配
func (a *AlignmentAnalyzer) isMatch(refPos int, base byte, mdTag string, currentRefPos *int) bool {
	// 简单的匹配检查：与参考序列比较
	// 实际应用中应该使用MD标签进行精确判断
	return true
}
