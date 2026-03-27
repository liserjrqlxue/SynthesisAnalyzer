package stats

import "io"

// ReadType 表示read的类型
type ReadType int

const (
	ReadTypeMatch              ReadType = iota // 匹配
	ReadTypeInsert                             // 插入
	ReadTypeDelete                             // 缺失
	ReadTypeSubstitution                       // 替换
	ReadTypeInsertDelete                       // 插入+缺失
	ReadTypeInsertSubstitution                 // 插入+替换
	ReadTypeDeleteSubstitution                 // 缺失+替换
	ReadTypeAll                                // 插入+缺失+替换
)

// ReadTypeNames ReadType对应的名称
var ReadTypeNames = map[ReadType]string{
	ReadTypeMatch:              "匹配",
	ReadTypeInsert:             "插入",
	ReadTypeDelete:             "缺失",
	ReadTypeSubstitution:       "替换",
	ReadTypeInsertDelete:       "插入+缺失",
	ReadTypeInsertSubstitution: "插入+替换",
	ReadTypeDeleteSubstitution: "缺失+替换",
	ReadTypeAll:                "插入+缺失+替换",
}

// 新增：缺失细分类
type DeletionSubtype int

const (
	Del1 DeletionSubtype = iota // 长度1
	Del2                        // 长度2
	Del3                        // 长度>2
)

// 新增：插入细分类
type InsertionSubtype int

const (
	Dup1   InsertionSubtype = iota // 长度1，与-1或+1碱基一致
	Dup2                           // 长度2，插入序列一致且与-1或+1碱基一致
	DupDup                         // 长度2，插入序列不一致，第一位与-1一致，第二位与+1一致 -> 2个Dup1
	Ins1                           // 长度1，非Dup1
	Ins2                           // 长度2，非Dup2/DupDup
	Ins3                           // 长度>2
)

// 新增：替换细分类
type SubstitutionSubtype int

const (
	DupDel        SubstitutionSubtype = iota // 与-1位相同
	DelDup                                   // 与+1位相同
	MismatchA_G                              // A>G
	MismatchC_T                              // C>T
	OtherMismatch                            // 其他
)

// 定义细分类名称映射
var DeleteNames = map[DeletionSubtype]string{
	Del1: "Del1",
	Del2: "Del2",
	Del3: "Del3",
}

var InsertNames = map[InsertionSubtype]string{
	Dup1:   "Dup1",
	Dup2:   "Dup2",
	DupDup: "DupDup",
	Ins1:   "Ins1",
	Ins2:   "Ins2",
	Ins3:   "Ins3",
}

var SubstNames = map[SubstitutionSubtype]string{
	DupDel:        "DupDel",
	DelDup:        "DelDup",
	MismatchA_G:   "MismatchA>G",
	MismatchC_T:   "MismatchC>T",
	OtherMismatch: "OtherMismatch",
}

func (stats *MutationStats) WriteSubtype(writer io.Writer) {
	totalEvents := stats.TotalInsertEventCount + stats.TotalDeleteEventCount + stats.TotalSubstitutionEventCount

	// 缺失
	for st := Del1; st <= Del3; st++ {
		name := "D:" + DeleteNames[st]
		count := stats.DeleteSubtypeEvents[st]
		write1line(writer, name, count, stats.TotalGoodAlignedReads, stats.TotalRefLengthGoodAligned, totalEvents)
	}

	// 插入
	for st := Dup1; st <= Ins3; st++ {
		if st == DupDup {
			continue
		}
		name := "I:" + InsertNames[st]
		count := stats.InsertSubtypeEvents[st]
		write1line(writer, name, count, stats.TotalGoodAlignedReads, stats.TotalRefLengthGoodAligned, totalEvents)
	}

	// 替换
	for st := DupDel; st <= OtherMismatch; st++ {
		var name = "X:" + SubstNames[st]
		var count = stats.SubstitutionSubtypeEvents[st]
		write1line(writer, name, count, stats.TotalGoodAlignedReads, stats.TotalRefLengthGoodAligned, totalEvents)
	}

}

// ClassifySubstitution 使用参考序列判定替换细分类
func (mutation SubstitutionInfo) ClassifySubstitution(refSeq string) SubstitutionSubtype {
	var (
		// pos 是 1-based 位置
		pos       = mutation.Position
		refBase   = mutation.Ref
		altBase   = mutation.Alt
		refMinus1 = "N"
		refPlus1  = "N"
	)

	if pos-2 >= 0 && pos-2 < len(refSeq) {
		refMinus1 = refSeq[pos-2 : pos-1]
	}
	if pos < len(refSeq) {
		refPlus1 = refSeq[pos : pos+1]
	}

	if altBase == refMinus1 {
		return DupDel
	}
	if altBase == refPlus1 {
		return DelDup
	}
	if refBase == "A" && altBase == "G" {
		return MismatchA_G
	}
	if refBase == "C" && altBase == "T" {
		return MismatchC_T
	}
	return OtherMismatch
}
