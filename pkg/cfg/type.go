package cfg

import "github.com/biogo/hts/sam"

// ReadType 表示read的类型
type ReadType int

const (
	ReadTypeMatch              ReadType = iota // 匹配
	ReadTypeMatchClip                          // 剪辑
	ReadTypeInsert                             // 插入
	ReadTypeDelete                             // 缺失
	ReadTypeSubstitution                       // 替换
	ReadTypeInsertDelete                       // 插入+缺失
	ReadTypeInsertSubstitution                 // 插入+替换
	ReadTypeDeleteSubstitution                 // 缺失+替换
	ReadTypeAll                                // 插入+缺失+替换
	ReadTypeOther                              // 其他组合或无法解析的reads
)

// ReadTypeNames ReadType对应的名称
var ReadTypeNames = map[ReadType]string{
	ReadTypeMatch:              "匹配",
	ReadTypeMatchClip:          "剪辑",
	ReadTypeInsert:             "插入",
	ReadTypeDelete:             "缺失",
	ReadTypeSubstitution:       "替换",
	ReadTypeInsertDelete:       "插入+缺失",
	ReadTypeInsertSubstitution: "插入+替换",
	ReadTypeDeleteSubstitution: "缺失+替换",
	ReadTypeAll:                "插入+缺失+替换",
	ReadTypeOther:"其它",
}

// Order 读取顺序
var Order = []string{
	ReadTypeNames[ReadTypeMatch],
	ReadTypeNames[ReadTypeMatchClip],
	ReadTypeNames[ReadTypeInsert],
	ReadTypeNames[ReadTypeDelete],
	ReadTypeNames[ReadTypeSubstitution],
	ReadTypeNames[ReadTypeInsertDelete],
	ReadTypeNames[ReadTypeInsertSubstitution],
	ReadTypeNames[ReadTypeDeleteSubstitution],
	ReadTypeNames[ReadTypeAll],
	"InsertReads",
	"DeleteReads",
	"SubstitutionReads",
	"ReadsWithMutations",
	"GoodAlignedReads",
}

// AnalyzeReadType 分析read的类型 - 更新版本，考虑M操作中的错配
func AnalyzeReadType(read *sam.Record) ReadType {
	hasInsert := false
	hasDelete := false
	hasSubstitution := false
	hasClip := false

	// 1. 检查CIGAR操作
	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()

		switch op {
		case sam.CigarInsertion: // I: 插入
			hasInsert = true
		case sam.CigarDeletion, sam.CigarSkipped: // D, N: 缺失或跳过
			hasDelete = true
		case sam.CigarMismatch: // X: 替换
			hasSubstitution = true
		case sam.CigarSoftClipped, sam.CigarHardClipped: // S, H: 剪辑
			hasClip = true
		}
	}

	// 2. 检查MD标签中的错配（包括M操作中的错配）
	mdStr, hasMD := GetMD(read)
	if hasMD {
		// 解析MD字符串检查错配
		mdHasDelete, mdHasSubstitution := CheckMismatchInMD(mdStr)
		if mdHasDelete {
			hasDelete = true
		}
		if mdHasSubstitution {
			hasSubstitution = true
		}
	}

	// 3. 根据组合确定类型
	if !hasInsert && !hasDelete && !hasSubstitution {
		if hasClip {
			return ReadTypeMatchClip
		}
		return ReadTypeMatch
	} else if hasInsert && !hasDelete && !hasSubstitution {
		return ReadTypeInsert
	} else if !hasInsert && hasDelete && !hasSubstitution {
		return ReadTypeDelete
	} else if !hasInsert && !hasDelete && hasSubstitution {
		return ReadTypeSubstitution
	} else if hasInsert && hasDelete && !hasSubstitution {
		return ReadTypeInsertDelete
	} else if hasInsert && !hasDelete && hasSubstitution {
		return ReadTypeInsertSubstitution
	} else if !hasInsert && hasDelete && hasSubstitution {
		return ReadTypeDeleteSubstitution
	} else if hasInsert && hasDelete && hasSubstitution {
		return ReadTypeAll
	}

	if hasClip {
		return ReadTypeMatchClip
	}
	return ReadTypeMatch
}

// DeletionInfo 单条缺失信息
type DeletionInfo struct {
	Length   int             // 缺失长度
	Bases    string          // 缺失的碱基序列（如果长度为1）
	Position int             // 缺失位置（1-based）
	Subtype  DeletionSubtype // 细分类
}

// 新增：缺失细分类
type DeletionSubtype int

const (
	Del1 DeletionSubtype = iota // 长度1
	Del2                        // 长度2
	Del3                        // 长度>2
)

// 新增：缺失细分类判定
func ClassifyDeletion(length int) DeletionSubtype {
	switch length {
	case 1:
		return Del1
	case 2:
		return Del2
	default: // >2
		return Del3
	}
}
