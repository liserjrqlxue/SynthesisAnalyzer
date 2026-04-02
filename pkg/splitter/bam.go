package splitter

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"SynthesisAnalyzer/pkg/cfg"

	"github.com/biogo/hts/bam"
	"github.com/biogo/hts/sam"
)

// 分析BAM文件，统计合成成功率
func analyzeBamFile(bamFile string, sample *cfg.Sample, mapQThreshold int) (*cfg.SampleAlignment, error) {
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
	positionStats := make([]cfg.PositionStat, sample.ReferenceLen)
	for i := range positionStats {
		positionStats[i].Position = i + 1 // 1-based position
	}

	// 初始化read类型计数器
	readTypeCounts := make(map[cfg.ReadType]int)

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
		if int(read.MapQ) < mapQThreshold {
			continue
		}

		mappedReads++

		// 解析MD标签
		mdTag := ""
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
		parseAlignmentWithDebug(read.Cigar, mdTag, string(read.Seq.Seq), &positionStats, &totalMatches, &totalMismatches, read.Name)
		mainType := cfg.AnalyzeReadType(read)
		readTypeCounts[mainType]++
	}

	// 计算汇总统计
	summary := &cfg.AlignmentSummary{
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

	return &cfg.SampleAlignment{
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
func (a *AlignmentAnalyzer) isMatch(refPos int, mdTag string, currentRefPos *int) bool {
	return a.isMatchSafe(refPos, mdTag, currentRefPos)
}

// 检查是否匹配的完整实现
func (a *AlignmentAnalyzer) isMatchSafe(refPos int, mdTag string, currentRefPos *int) bool {
	if refPos < 0 || currentRefPos == nil {
		return false
	}

	// 如果 MD 标签为空，假设为匹配
	if mdTag == "" {
		return true
	}

	// 解析 MD 标签
	return a.parseMDTagForPosition(mdTag, refPos, currentRefPos)
}

// 解析 MD 标签判断指定位置是否匹配
func (a *AlignmentAnalyzer) parseMDTagForPosition(mdTag string, refPos int, currentRefPos *int) bool {
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
func parseMDTag(mdTag string) *MDTagParser {
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
			parser.positionInfo[pos] = baseToCode(refBase)
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
func baseToCode(base byte) byte {
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
func isMatchWithParser(refPos int, parser *MDTagParser) bool {
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
func parseAlignmentWithErrors(cigar sam.Cigar, mdTag, seq string, positionStats *[]cfg.PositionStat, totalMatches, totalMismatches *int64) {
	refPos := 0
	seqPos := 0
	seqLen := len(seq)

	// 预解析 MD 标签
	mdParser := parseMDTag(mdTag)

	// 解析 CIGAR
	for _, cigarOp := range cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch:
			for j := 0; j < length; j++ {
				// 边界检查
				if seqPos >= seqLen {
					return
				}

				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++

					// 使用预解析的 MD 标签判断是否匹配
					if op == sam.CigarEqual || (mdParser == nil || isMatchWithParser(refPos, mdParser)) {
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

		case sam.CigarDeletion, sam.CigarSkipped:
			// 缺失或跳过，只消耗参考序列
			for j := 0; j < length; j++ {
				if refPos < len(*positionStats) {
					(*positionStats)[refPos].TotalReads++
					(*positionStats)[refPos].DeletionCount++
				}
				refPos++
			}

		case sam.CigarInsertion:
			// 插入
			if refPos > 0 && refPos-1 < len(*positionStats) {
				(*positionStats)[refPos-1].InsertionCount++
			}
			seqPos += length

		case sam.CigarSoftClipped:
			// 软裁剪，只消耗序列
			seqPos += length

		case sam.CigarHardClipped:
			// 硬裁剪，不消耗序列或参考序列
			continue
		default:
			slog.Error("未知的CIGAR操作符", "op", op)
		}
	}
}

// 新增：验证read类型统计的辅助函数
func (a *AlignmentAnalyzer) ValidateReadTypeCounts(counts map[cfg.ReadType]int, mappedReads int64) bool {
	if counts == nil {
		return false
	}

	var calculatedTotal int64
	for _, count := range counts {
		calculatedTotal += int64(count)
	}
	return calculatedTotal == mappedReads
}

// 带调试信息的版本
func parseAlignmentWithDebug(cigar sam.Cigar, mdTag, seq string, positionStats *[]cfg.PositionStat, totalMatches, totalMismatches *int64, readID string) {

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("解析记录时panic: %v\n", r)
			fmt.Printf("Read ID: %s\n", readID)
			fmt.Printf("CIGAR: %s\n", cigar.String())
			fmt.Printf("MD tag: %s\n", mdTag)
			fmt.Printf("Seq length: %d\n", len(seq))
			fmt.Printf("Seq (前100字符): %s\n", safeSubstr(seq, 0, 100))
		}
	}()

	// 调用正常的解析函数
	parseAlignmentWithErrors(cigar, mdTag, seq, positionStats, totalMatches, totalMismatches)
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
