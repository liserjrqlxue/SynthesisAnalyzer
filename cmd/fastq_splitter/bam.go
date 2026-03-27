package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/biogo/hts/bam"
)

// 分析BAM文件，统计合成成功率
func (a *AlignmentAnalyzer) analyzeBamFile(bamFile string, sample *SampleInfo) (*SampleAlignment, error) {
	// 打开BAM文件
	f, err := os.Open(bamFile)
	if err != nil {
		return nil, fmt.Errorf("打开BAM文件失败: %v", err)
	}
	defer f.Close()

	// 创建BAM读取器
	br, err := bam.NewReader(f, 0)
	if err != nil {
		return nil, fmt.Errorf("创建BAM读取器失败: %v", err)
	}
	defer br.Close()

	// 初始化位置统计
	positionStats := make([]PositionStat, sample.ReferenceLen)
	for i := range positionStats {
		positionStats[i].Position = i + 1 // 1-based position
	}

	// 初始化read类型计数器
	readTypeCounts := &ReadTypeCounts{}

	totalReads := int64(0)
	mappedReads := int64(0)
	totalMismatches := int64(0)
	totalMatches := int64(0)

	// 遍历BAM记录
	for {
		read, err := br.Read()
		if err != nil {
			break // 读取完成
		}

		totalReads++

		// 检查比对质量
		if int(read.MapQ) < a.config.MapQThreshold {
			continue
		}

		mappedReads++

		// 解析CIGAR和MD标签
		cigar := read.Cigar.String()
		mdTag := ""

		// 查找MD标签
		if mdTagValue, ok := read.Tag([]byte("MD")); ok {
			mdTag = mdTagValue.String()
			// 移除 "MD:Z:"
			if len(mdTag) > 5 && mdTag[4] == ':' {
				mdTag = mdTag[5:]
			} else {
				mdTag = strings.TrimPrefix(mdTag, "MD:Z:")
			}
		}

		// 解析比对结果并获取错误类型
		errorFlags := a.parseAlignmentWithErrors(
			cigar, mdTag, string(read.Seq.Seq), sample,
			&positionStats, &totalMatches, &totalMismatches,
		)

		// 根据错误类型分类read
		a.classifyRead(errorFlags, readTypeCounts)
	}

	// 计算汇总统计
	summary := &AlignmentSummary{
		TotalReads:      totalReads,
		MappedReads:     mappedReads,
		AverageCoverage: 0,
		AverageIdentity: 0,
		ReadTypeCounts:  readTypeCounts, // 添加类型统计
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
		SampleName:     sample.Name,
		ReferenceSeq:   sample.ReferenceSeq,
		ReferenceLen:   sample.ReferenceLen,
		PositionStats:  positionStats,
		Summary:        summary,
		BamFile:        bamFile,
		BamIndex:       bamFile + ".bai",
		ReadTypeCounts: readTypeCounts,
	}, nil
}

/* // 解析比对结果
func (a *AlignmentAnalyzer) parseAlignment(cigar, mdTag, seq string, sample *SampleInfo,
	positionStats *[]PositionStat, totalMatches, totalMismatches *int64) error {

	defer func() {
		if r := recover(); r != nil {
			log.Printf("解析比对记录时发生panic: %v\n", r)
			log.Printf("CIGAR: %s, MD: %s, Seq长度: %d\n", cigar, mdTag, len(seq))
		}
	}()

	refPos := 0
	seqPos := 0
	seqLen := len(seq)

	// 验证输入
	if len(cigar) == 0 {
		return fmt.Errorf("CIGAR字符串为空")
	}

	if seqLen == 0 {
		return fmt.Errorf("序列为空")
	}

	// 记录原始值，用于调试
	// originalSeqLen := seqLen
	originalCigar := cigar

	// 预检查：确保CIGAR操作消耗的序列长度不超过实际序列长度
	totalSeqOps := a.calculateSequenceOperations(cigar)
	if totalSeqOps > seqLen {
		log.Printf("警告: CIGAR操作需要的序列长度(%d)大于实际序列长度(%d)，跳过该记录\n",
			totalSeqOps, seqLen)
		return fmt.Errorf("CIGAR操作需要的序列长度(%d)大于实际序列长度(%d)，跳过该记录\n",
			totalSeqOps, seqLen)
	}

	// 解析CIGAR
	i := 0
	cigarIndex := 0

	for i < len(cigar) {
		cigarIndex++
		if cigarIndex > 1000 {
			log.Printf("警告: CIGAR解析循环可能陷入无限循环，CIGAR: %s\n", cigar)
			break
		}

		// 解析数字
		num := 0
		for i < len(cigar) && cigar[i] >= '0' && cigar[i] <= '9' {
			num = num*10 + int(cigar[i]-'0')
			i++
		}

		if i >= len(cigar) {
			return fmt.Errorf("CIGAR格式错误: 数字后没有操作符")

		}

		op := cigar[i]
		i++

		// 根据操作符处理
		switch op {
		case 'M', '=', 'X':
			// 匹配/不匹配/错配，消耗序列和参考序列
			for j := 0; j < num; j++ {
				if seqPos >= seqLen {
					return fmt.Errorf("序列位置越界: seqPos=%d, seqLen=%d, CIGAR=%s",
						seqPos, seqLen, originalCigar)
				}
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++

					// 使用安全的序列访问
					seqChar := seq[seqPos]
					if a.isMatchSafe(refPos, seqChar, mdTag) {
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
		case 'I':
			// 插入，只消耗序列
			if refPos > 0 && refPos-1 < len(*positionStats) {
				(*positionStats)[refPos-1].InsertionCount++
			}
			if seqPos+num > seqLen {
				return fmt.Errorf("插入操作导致序列越界: seqPos=%d, num=%d, seqLen=%d",
					seqPos, num, seqLen)
			}
			seqPos += num
		case 'D', 'N':
			// 缺失或跳过，只消耗参考序列
			for j := 0; j < num; j++ {
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++
					(*positionStats)[refPos].DeletionCount++
				}
				refPos++
			}
		case 'S':
			// 软裁剪，只消耗序列
			if seqPos+num > seqLen {
				return fmt.Errorf("软裁剪导致序列越界: seqPos=%d, num=%d, seqLen=%d",
					seqPos, num, seqLen)
			}
			seqPos += num

		case 'H', 'P':
			// 硬裁剪或填充，不消耗序列或参考序列
			continue

		default:
			return fmt.Errorf("未知的CIGAR操作符: %c", op)
		}
	}

	// 最终验证
	if seqPos != seqLen {
		log.Printf("警告: 序列未完全消耗: seqPos=%d, seqLen=%d, CIGAR=%s\n",
			seqPos, seqLen, originalCigar)
	}

	return nil
}
*/

// 添加辅助函数，计算CIGAR操作消耗的序列长度
func (a *AlignmentAnalyzer) calculateSequenceOperations(cigar string) int {
	totalSeqOps := 0
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

		// 消耗序列的操作：M, I, S, =, X
		switch op {
		case 'M', 'I', 'S', '=', 'X':
			totalSeqOps += num
		}
	}

	return totalSeqOps
}

// 检查是否匹配
// 简化的 isMatch 函数，调用 isMatchSafe
func (a *AlignmentAnalyzer) isMatch(refPos int, base byte, mdTag string, currentRefPos *int) bool {
	return a.isMatchSafe(refPos, base, mdTag, currentRefPos)
}

// 检查是否匹配的完整实现
func (a *AlignmentAnalyzer) isMatchSafe(refPos int, seqBase byte, mdTag string, currentRefPos *int) bool {
	if refPos < 0 || currentRefPos == nil {
		return false
	}

	// 如果 MD 标签为空，假设为匹配
	if mdTag == "" {
		return true
	}

	// 解析 MD 标签
	return a.parseMDTagForPosition(mdTag, refPos, seqBase, currentRefPos)
}

// 解析 MD 标签判断指定位置是否匹配
func (a *AlignmentAnalyzer) parseMDTagForPosition(mdTag string, refPos int, seqBase byte, currentRefPos *int) bool {
	// MD 标签格式说明：
	// - 数字表示匹配的连续长度
	// - 字母表示参考序列中的碱基（错配）
	// - ^ 后跟字母表示删除的参考碱基
	// 示例: "MD:Z:10A5^AC10" 表示10个匹配，然后一个A（参考）与测序碱基错配，然后5个匹配，然后删除AC，然后10个匹配

	// 为了判断特定位置 refPos 是否匹配，我们需要模拟 MD 标签的解析过程

	pos := 0 // 当前参考位置
	i := 0   // MD 标签字符串索引
	mdLen := len(mdTag)

	for i < mdLen && pos <= refPos {
		if mdTag[i] >= '0' && mdTag[i] <= '9' {
			// 解析数字（匹配长度）
			num := 0
			for i < mdLen && mdTag[i] >= '0' && mdTag[i] <= '9' {
				num = num*10 + int(mdTag[i]-'0')
				i++
			}

			// 检查 refPos 是否在这个匹配块内
			if refPos >= pos && refPos < pos+num {
				// refPos 在匹配块内，所以是匹配
				if currentRefPos != nil {
					*currentRefPos = pos + num // 更新到匹配块结束
				}
				return true
			}
			pos += num

		} else if i < mdLen && mdTag[i] == '^' {
			// 删除操作
			i++ // 跳过 '^'

			// 跳过删除的碱基
			for i < mdLen && ((mdTag[i] >= 'A' && mdTag[i] <= 'Z') ||
				(mdTag[i] >= 'a' && mdTag[i] <= 'z')) {
				// 检查 refPos 是否在这个删除位置
				if pos == refPos {
					// 这个位置是删除，序列中没有对应碱基
					if currentRefPos != nil {
						*currentRefPos = pos + 1
					}
					return false
				}
				pos++
				i++
			}

		} else if i < mdLen && ((mdTag[i] >= 'A' && mdTag[i] <= 'Z') ||
			(mdTag[i] >= 'a' && mdTag[i] <= 'z')) {
			// 错配（参考碱基）
			// refBase := mdTag[i]
			i++

			// 检查是否是我们要的位置
			if pos == refPos {
				// 这个位置是错配
				if currentRefPos != nil {
					*currentRefPos = pos + 1
				}

				// 可选：验证测序碱基是否真的与参考碱基不同
				// 注意：MD 标签中的碱基是参考序列的碱基
				// 所以如果 seqBase 与 refBase 相同，应该是匹配，但 MD 标记为错配
				// 这可能是测序错误或其他情况，我们相信 MD 标签
				return false
			}
			pos++

		} else {
			// 未知字符，跳过
			i++
		}
	}

	// 如果走到这里，说明 refPos 超出了 MD 标签描述的范围
	// 这可能发生在 CIGAR 操作（如插入）不消耗参考位置的情况下
	// 或者 MD 标签不完整，保守地返回 true
	return true
}

type MDTagParser struct {
	// 存储每个参考位置的信息
	positionInfo map[int]byte // 0=匹配, 1=A, 2=C, 3=G, 4=T, 5=N, 255=删除
	maxPosition  int
}

// 预解析 MD 标签
func (a *AlignmentAnalyzer) parseMDTag(mdTag string) *MDTagParser {
	parser := &MDTagParser{
		positionInfo: make(map[int]byte),
		maxPosition:  0,
	}

	if mdTag == "" {
		return parser
	}

	pos := 0
	i := 0
	mdLen := len(mdTag)

	for i < mdLen {
		if mdTag[i] >= '0' && mdTag[i] <= '9' {
			// 匹配块
			num := 0
			for i < mdLen && mdTag[i] >= '0' && mdTag[i] <= '9' {
				num = num*10 + int(mdTag[i]-'0')
				i++
			}
			// 匹配块内所有位置都是匹配，不需要存储（默认为0）
			pos += num

		} else if i < mdLen && mdTag[i] == '^' {
			// 删除块
			i++ // 跳过 '^'

			for i < mdLen && ((mdTag[i] >= 'A' && mdTag[i] <= 'Z') ||
				(mdTag[i] >= 'a' && mdTag[i] <= 'z')) {
				// 标记为删除
				parser.positionInfo[pos] = 255
				pos++
				i++
			}

		} else if i < mdLen && ((mdTag[i] >= 'A' && mdTag[i] <= 'Z') ||
			(mdTag[i] >= 'a' && mdTag[i] <= 'z')) {
			// 错配
			refBase := mdTag[i]
			i++

			// 存储参考碱基
			parser.positionInfo[pos] = a.baseToCode(refBase)
			pos++

		} else {
			// 未知字符
			i++
		}
	}

	parser.maxPosition = pos - 1
	return parser
}

// 碱基字符转编码
func (a *AlignmentAnalyzer) baseToCode(base byte) byte {
	switch base {
	case 'A', 'a':
		return 1
	case 'C', 'c':
		return 2
	case 'G', 'g':
		return 3
	case 'T', 't':
		return 4
	case 'N', 'n':
		return 5
	default:
		return 0 // 未知碱基视为匹配
	}
}

// 使用预解析的 MD 标签检查匹配
func (a *AlignmentAnalyzer) isMatchWithParser(refPos int, seqBase byte, parser *MDTagParser) bool {
	if refPos < 0 || refPos > parser.maxPosition {
		return true // 超出范围，保守返回匹配
	}

	code, exists := parser.positionInfo[refPos]
	if !exists {
		// 位置不在 map 中，说明是匹配
		return true
	}

	if code == 255 {
		// 删除位置，不应该被调用（应该在 CIGAR 的 D 操作中处理）
		return false
	}

	// 错配位置
	return false
}

// 解析比对结果（改进版）
func (a *AlignmentAnalyzer) parseAlignmentWithErrors(cigar, mdTag, seq string, sample *SampleInfo,
	positionStats *[]PositionStat, totalMatches, totalMismatches *int64) *readErrorFlags {

	flags := &readErrorFlags{}
	refPos := 0
	seqPos := 0
	seqLen := len(seq)

	// 预解析 MD 标签
	mdParser := a.parseMDTag(mdTag)

	// 解析 CIGAR
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
				// 边界检查
				if seqPos >= seqLen {
					fmt.Printf("错误: 序列位置越界 seqPos=%d, seqLen=%d\n", seqPos, seqLen)
					slog.Error("错误: 序列位置越界", "seqPos", seqPos, "seqLen", seqLen, "cigar", cigar, "mdTag", mdTag, "seq", seq)
					return flags
				}

				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++

					// 使用预解析的 MD 标签判断是否匹配
					if mdParser == nil || a.isMatchWithParser(refPos, seq[seqPos], mdParser) {
						(*positionStats)[refPos].MatchCount++
						(*totalMatches)++
					} else {
						(*positionStats)[refPos].MismatchCount++
						(*totalMismatches)++
						flags.hasMismatch = true
					}
				}
				refPos++
				seqPos++
			}

		case 'D', 'N':
			// 缺失或跳过，只消耗参考序列
			flags.hasDeletion = true
			for j := 0; j < num; j++ {
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++
					(*positionStats)[refPos].DeletionCount++
				}
				refPos++
			}

		case 'I': // 插入
			flags.hasInsertion = true
			if refPos > 0 && refPos-1 < len(*positionStats) {
				(*positionStats)[refPos-1].InsertionCount++
			}
			seqPos += num

		case 'S':
			// 软裁剪，只消耗序列
			flags.hasInsertion = true
			seqPos += num

		case '=', 'X': // 明确的匹配/错配
			for j := 0; j < num; j++ {
				if seqPos >= seqLen {
					fmt.Printf("错误: 序列位置越界 seqPos=%d, seqLen=%d\n", seqPos, seqLen)
					slog.Error("错误: 序列位置越界", "seqPos", seqPos, "seqLen", seqLen, "cigar", cigar, "mdTag", mdTag, "seq", seq)
					return flags
				}

				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++

					if op == '=' {
						(*positionStats)[refPos].MatchCount++
						(*totalMatches)++
					} else { // 'X'
						(*positionStats)[refPos].MismatchCount++
						(*totalMismatches)++
						flags.hasMismatch = true
					}
				}
				refPos++
				seqPos++
			}
		case 'H', 'P':
			// 硬裁剪或填充，不消耗序列或参考序列
			flags.hasInsertion = true
			continue
		default:
			slog.Error("未知的CIGAR操作符", "op", op)
		}
	}
	return flags

	// 验证：序列应该被完全消耗（除了被裁剪的部分）
	// 但硬裁剪（H）不影响序列长度，软裁剪（S）影响
	// 这里不强制验证，因为有S/H操作
}

// 新增：根据错误类型分类read
func (a *AlignmentAnalyzer) classifyRead(flags *readErrorFlags, counts *ReadTypeCounts) {
	if flags == nil {
		counts.Other++
		return
	}

	// 统计错误类型
	errorCount := 0
	if flags.hasMismatch {
		errorCount++
	}
	if flags.hasInsertion {
		errorCount++
	}
	if flags.hasDeletion {
		errorCount++
	}

	switch errorCount {
	case 0:
		// 完全正确的read
		counts.PerfectReads++
	case 1:
		// 只有一种错误
		if flags.hasMismatch {
			counts.MismatchOnly++
		} else if flags.hasInsertion {
			counts.InsertionOnly++
		} else if flags.hasDeletion {
			counts.DeletionOnly++
		}
	case 2:
		// 有两种错误
		if flags.hasMismatch && flags.hasInsertion {
			counts.MixedMismatchIns++
		} else if flags.hasMismatch && flags.hasDeletion {
			counts.MixedMismatchDel++
		} else if flags.hasInsertion && flags.hasDeletion {
			counts.MixedInsDel++
		} else {
			counts.Other++
		}
	case 3:
		// 三种错误都有
		counts.AllErrors++
	default:
		counts.Other++
	}
}

// 新增：验证read类型统计的辅助函数
func (a *AlignmentAnalyzer) ValidateReadTypeCounts(counts *ReadTypeCounts, mappedReads int64) bool {
	if counts == nil {
		return false
	}

	calculatedTotal := counts.PerfectReads + counts.MismatchOnly + counts.InsertionOnly +
		counts.DeletionOnly + counts.MixedMismatchIns + counts.MixedMismatchDel +
		counts.MixedInsDel + counts.AllErrors + counts.Other

	return calculatedTotal == mappedReads
}

// 带调试信息的版本
func (a *AlignmentAnalyzer) parseAlignmentWithDebug(cigar, mdTag, seq string, sample *SampleInfo,
	positionStats *[]PositionStat, totalMatches, totalMismatches *int64, readID string) {

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("解析记录时panic: %v\n", r)
			fmt.Printf("Read ID: %s\n", readID)
			fmt.Printf("CIGAR: %s\n", cigar)
			fmt.Printf("MD tag: %s\n", mdTag)
			fmt.Printf("Seq length: %d\n", len(seq))
			fmt.Printf("Seq (前100字符): %s\n", safeSubstr(seq, 0, 100))
		}
	}()

	// 调用正常的解析函数
	a.parseAlignmentWithErrors(cigar, mdTag, seq, sample, positionStats, totalMatches, totalMismatches)
}

func safeSubstr(s string, start, length int) string {
	if start < 0 || start >= len(s) {
		return ""
	}
	end := start + length
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}
