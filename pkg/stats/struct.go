package stats

import (
	"log/slog"

	"github.com/biogo/hts/sam"
)

// ========== 新增：位置详细统计结构 ==========
type PositionDetail struct {
	Base                byte // ACGTN
	Depth               int
	MatchPure           int
	MatchWithIns        int
	MismatchPure        int
	MismatchWithIns     int
	Insertion           int
	Deletion            int
	Del1                int
	PerfectReadsCount   int
	PerfectUptoPosCount int
	AlignedSum          int // 覆盖该位置的所有样品的 aligned reads 之和
	TotalSum            int // 覆盖该位置的所有样品的 total reads 之和

	NCorrect  int
	N1Correct int
}

// InsertSubtype 插入子类型（多条插入）
type InsertSubtype struct {
	Insertions []InsertionInfo // 每条插入的信息
}

func (subtype *InsertSubtype) Classify(read *sam.Record, refSeq, seq string, refPos, readPos, length int) {
	insertionSubtype := classifyInsertion(refSeq, seq, refPos, readPos, length)
	// DupDup 转换为 2个Dup1
	if insertionSubtype == DupDup {
		insertion1 := InsertionInfo{
			Bases:   seq[readPos:min(readPos+1, len(seq))],
			Length:  1,
			Subtype: Dup1,
		}
		insertion2 := InsertionInfo{
			Bases:   seq[readPos+1 : min(readPos+2, len(seq))],
			Length:  1,
			Subtype: Dup1,
		}
		subtype.Insertions = append(subtype.Insertions, insertion1, insertion2)
	} else {
		insertion := InsertionInfo{
			Bases:   seq[readPos:min(readPos+length, len(seq))],
			Length:  length,
			Subtype: insertionSubtype,
		}
		subtype.Insertions = append(subtype.Insertions, insertion)
	}
}

// InsertionInfo 单条插入信息
type InsertionInfo struct {
	Length  int              // 插入长度
	Bases   string           // 插入的碱基序列
	Subtype InsertionSubtype // 细分类
}

// DeleteSubtype 缺失子类型
type DeleteSubtype struct {
	Deletions []DeletionInfo // 每条缺失的信息
}

func (subtype *DeleteSubtype) Classify(read *sam.Record, refPos, length int) {
	// 记录缺失长度
	deletion := DeletionInfo{
		Length:   length,
		Position: refPos + 1, // 转换为1-based
		Subtype:  classifyDeletion(length),
	}
	subtype.Deletions = append(subtype.Deletions, deletion)
}

// DeletionInfo 单条缺失信息
type DeletionInfo struct {
	Length   int             // 缺失长度
	Bases    string          // 缺失的碱基序列（如果长度为1）
	Position int             // 缺失位置（1-based）
	Subtype  DeletionSubtype // 细分类
}

// 替换信息结构（用于记录替换细分类）
type SubstitutionInfo struct {
	Position int
	Ref      string
	Alt      string
	Subtype  SubstitutionSubtype
}

// SubstitutionSubtype 替换细分类
type SubstituteSubtype struct {
	Substitutions []SubstitutionInfo // 每条替换的信息
}

func (subtype *SubstituteSubtype) Classify(read *sam.Record, mdMap map[int]string, refSeq, seq string, refPos, readPos, length int, op sam.CigarOpType) {
	refSeqLen := len(refSeq)

	for i := range length {
		refBase := byte('N')
		currentRefPos := refPos + i

		if refSeq != "" && currentRefPos >= 0 && currentRefPos < refSeqLen {
			refBase = refSeq[currentRefPos]
		} else if mdMap != nil {
			if base, ok := mdMap[currentRefPos]; ok {
				refBase = base[0]
			}
		}

		altBase := byte('N')
		currentReadPos := readPos + i
		if currentReadPos < len(seq) {
			altBase = seq[currentReadPos]
		}

		if op == sam.CigarMismatch || refBase != altBase {
			mutation := SubstitutionInfo{
				Position: currentRefPos + 1,
				Ref:      string(refBase),
				Alt:      string(altBase),
			}
			mutation.Subtype = mutation.ClassifySubstitution(refSeq)
			subtype.Substitutions = append(subtype.Substitutions, mutation)
			slog.Debug("mutation", "position", mutation.Position, "ref", mutation.Ref, "alt", mutation.Alt, "subtype", mutation.Subtype)
		}
	}

}

// ReadDetailedInfo 详细的read信息
type ReadDetailedInfo struct {
	MainType       ReadType
	InsertSub      *InsertSubtype
	DeleteSub      *DeleteSubtype
	Substitutions  []SubstitutionInfo // 该read中的所有突变，兼容替换细分类列表
	SubtypeTags    []string           // 该read的所有细分类标签（去重后排序用）
	CombinationKey string             // 细分类组合唯一键（排序后拼接）
}

// 定义累加器结构
type SumPct struct {
	SumGood    float64
	SumAligned float64
	SumTotal   float64
	Count      int
}
