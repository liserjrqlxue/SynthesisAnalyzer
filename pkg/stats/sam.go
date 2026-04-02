package stats

import (
	"sort"
	"strings"

	"SynthesisAnalyzer/pkg/cfg"

	"github.com/biogo/hts/sam"
)

// analyzeSubstitutionSubtype 使用参考序列和MD标签解析突变
func analyzeSubstitutionSubtype(read *sam.Record, refSeq string, mdMap map[int]string) *SubstituteSubtype {
	subtype := &SubstituteSubtype{}
	refStart := int(read.Pos)
	seq := string(read.Seq.Expand())
	if len(seq) == 0 {
		return subtype
	}

	refPos := refStart
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch:
			subtype.Classify(read, mdMap, refSeq, seq, refPos, readPos, length, op)

			refPos += length
			readPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped:
			readPos += length
		case sam.CigarDeletion, sam.CigarSkipped:
			refPos += length
		}
	}

	return subtype
}

// analyzeReadDetailedInfo 分析详细的read信息
func analyzeReadDetailedInfo(read *sam.Record, mdMap map[int]string, mdStr, refSeq string) ReadDetailedInfo {
	var info ReadDetailedInfo

	// 基本类型分析
	info.MainType = cfg.AnalyzeReadType(read)

	hasInsertion, hasDelete, _ := CheckCigar(read)

	// 分析插入子类型
	if hasInsertion {
		info.InsertSub = analyzeInsertSubtype(read, refSeq)
	}

	// 分析缺失子类型
	if hasDelete {
		info.DeleteSub = analyzeDeleteSubtype(read, mdStr)
	}

	// 解析突变
	info.SubstituteSub = analyzeSubstitutionSubtype(read, refSeq, mdMap)
	return info
}

// analyzeInsertSubtype 分析插入子类型，使用参考序列
func analyzeInsertSubtype(read *sam.Record, refSeq string) *InsertSubtype {
	subtype := &InsertSubtype{}
	seq := string(read.Seq.Expand())
	refPos := int(read.Pos) // 0-basedd
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
			readPos += length
		case sam.CigarInsertion: // I
			subtype.Classify(read, refSeq, seq, refPos, readPos, length)

			readPos += length
		case sam.CigarSoftClipped: // S
			readPos += length
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			refPos += length
		}
	}

	return subtype
}

// analyzeDeleteSubtype 分析缺失子类型 - 添加细分类
func analyzeDeleteSubtype(read *sam.Record, mdStr string) *DeleteSubtype {
	refStart := int(read.Pos) // 0-based
	subtype := &DeleteSubtype{}

	// 从MD字符串解析缺失的碱基和位置
	if mdStr != "" {
		// 解析MD字符串获取缺失的碱基和位置
		deletedInfos := cfg.ParseDeletionInfoFromMD(mdStr, refStart)
		for i := range deletedInfos {
			deletedInfos[i].Subtype = cfg.ClassifyDeletion(deletedInfos[i].Length)
		}
		subtype.Deletions = deletedInfos
		return subtype
	}

	// 如果没有MD标签，只从CIGAR获取长度和估算位置
	refPos := refStart
	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped: // I, S
			// 插入，不增加参考位置
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			subtype.Classify(read, refPos, length)
			refPos += length
		}
	}
	return subtype
}

// 新增：生成read的细分类组合键
func buildSubtypeCombinationKey(insertSubtypes map[InsertionSubtype]bool, deleteSubtypes map[cfg.DeletionSubtype]bool, substSubtypes map[SubstitutionSubtype]bool) string {
	var tags []string
	// 插入
	for st := range insertSubtypes {
		tags = append(tags, InsertNames[st])
	}
	// 缺失
	for st := range deleteSubtypes {
		tags = append(tags, DeleteNames[st])
	}
	// 替换
	for st := range substSubtypes {
		tags = append(tags, SubstNames[st])
	}
	sort.Strings(tags)
	return strings.Join(tags, "_")
}

// classifyInsertion 判定插入细分类
// 依赖 参考序列上下文（-1/+1位置）
func classifyInsertion(refSeq, seq string, refPos, readPos, length int) InsertionSubtype {
	// refPos 是插入前的参考位置（0-based）
	// 获取-1和+1位置的碱基（如果存在）
	leftBase := byte('N')
	if refPos >= 0 && refPos < len(refSeq) {
		leftBase = refSeq[refPos]
	}
	rightBase := byte('N')
	if refPos+1 >= 0 && refPos+1 < len(refSeq) {
		rightBase = refSeq[refPos+1]
	}

	switch length {
	case 1:
		// 长度1
		base := seq[readPos]
		if base == leftBase || base == rightBase {
			return Dup1
		}
		return Ins1
	case 2:
		// 长度2
		b1 := seq[readPos]
		b2 := seq[readPos+1]
		// 插入序列是否一致
		if b1 == b2 {
			// 一致序列，检查是否与-1或+1相同
			if b1 == leftBase || b1 == rightBase {
				return Dup2
			}
		} else {
			// 不一致序列，检查是否第一位与-1一致，第二位与+1一致
			if b1 == leftBase && b2 == rightBase {
				return DupDup
			}
		}
		return Ins2
	default:
		// 长度 >2
		return Ins3
	}
}

// CheckCigar 检查CIGAR中是否有插入、缺失、替换
func CheckCigar(read *sam.Record) (hasInsertion bool, hasDelete bool, hasSubstitution bool) {
	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()

		switch op {
		case sam.CigarInsertion: // I
			hasInsertion = true
		case sam.CigarDeletion, sam.CigarSkipped: // D, N: 缺失
			hasDelete = true
		case sam.CigarMismatch: // X: 替换
			hasSubstitution = true
		}
	}
	return
}
