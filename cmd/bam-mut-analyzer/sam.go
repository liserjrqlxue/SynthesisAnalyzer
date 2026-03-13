package main

import (
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/biogo/hts/sam"
)

// parseMutationsWithMD 使用参考序列和MD标签解析突变
func parseMutationsWithMD(read *sam.Record, refSeq string, mdMap map[int]string) (mutations []Mutation) {
	refStart := int(read.Pos) // 0-based起始位置
	seq := read.Seq.Expand()
	if len(seq) == 0 {
		return
	}

	// 获取MD标签
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	mdStr := ""
	if hasMD {
		mdStr = mdTag.String()
		mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
	}

	// 遍历CIGAR操作
	refPos := refStart // 0-based
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			for i := range length {
				// 获取参考碱基
				refBase := "N"
				// 优先使用参考序列
				if refSeq != "" && refPos+i >= 0 && refPos+i < len(refSeq) {
					refBase = string(refSeq[refPos+i])
				} else if mdMap != nil {
					// 否则使用 MD 映射
					if base, ok := mdMap[refPos+i]; ok {
						refBase = base
					}
				}

				// 获取 read 碱基
				altBase := "N"
				if readPos+i < len(seq) {
					altBase = string(seq[readPos+i])
				}

				if op == sam.CigarMismatch || refBase != altBase {
					mutation := Mutation{
						Position: refPos + i + 1, // 转换为1-based输出
						Ref:      refBase,
						Alt:      altBase,
					}
					mutation.Subtype = mutation.ClassifySubstitution(refSeq)
					mutations = append(mutations, mutation)
				}
			}

			// 更新位置
			refPos += length
			readPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped: // I, S
			readPos += length
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			refPos += length
		}
	}

	return
}

// parseMDToMap 解析MD字符串，返回位置(0-based)->参考碱基的映射（只包含错配）
func parseMDToMap(mdStr string, refStart int) map[int]string {
	result := make(map[int]string)

	if mdStr == "" {
		return result
	}

	// pos是0-based的位置（相对于参考序列起始）
	pos := refStart
	i := 0

	for i < len(mdStr) {
		// 读取数字（匹配长度）
		if isDigit(mdStr[i]) {
			j := i
			for j < len(mdStr) && isDigit(mdStr[j]) {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				i = j
				log.Fatalf("num Error[%s]:[%s]", numStr, mdStr)
				continue
			}

			// 匹配区域，不记录到映射中
			pos += num
			i = j
		} else if isBase(mdStr[i]) && mdStr[i] != '^' {
			// 错配（碱基）
			// 检查是否在删除标记后
			if i > 0 && mdStr[i-1] == '^' {
				// 在删除标记后，这是被删除的碱基，不是错配
				pos++
				i++
			} else {
				// 真正的错配
				result[pos] = string(mdStr[i])
				pos++
				i++
			}
		} else if mdStr[i] == '^' {
			// 删除
			i++
			// 跳过删除的碱基
			for i < len(mdStr) && isBase(mdStr[i]) {
				// 删除的碱基不记录到错配映射中
				pos++
				i++
			}
		} else {
			i++
		}
	}

	return result
}

// isBase 判断字符是否为有效碱基
func isBase(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// checkMismatchInMD 检查MD字符串中是否有错配和删除
func checkMismatchInMD(mdStr string) (bool, bool) {
	hasDelete := false
	hasSubstitution := false

	i := 0
	for i < len(mdStr) {
		if isDigit(mdStr[i]) {
			// 跳过数字
			i++
			for i < len(mdStr) && isDigit(mdStr[i]) {
				i++
			}
		} else if mdStr[i] == '^' {
			hasDelete = true
			i++
			// 跳过所有非数字字符（删除的碱基）
			for i < len(mdStr) && !isDigit(mdStr[i]) {
				i++
			}
		} else if isBase(mdStr[i]) {
			// 不是数字也不是'^'，就是错配碱基
			hasSubstitution = true
			i++
		} else {
			i++
		}
	}

	return hasDelete, hasSubstitution
}

// analyzeReadType 分析read的类型 - 更新版本，考虑M操作中的错配
func analyzeReadType(read *sam.Record) ReadType {
	hasInsert := false
	hasDelete := false
	hasSubstitution := false

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
		}
	}

	// 2. 检查MD标签中的错配（包括M操作中的错配）
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	if hasMD {
		mdStr := mdTag.String()
		mdStr = strings.TrimPrefix(mdStr, "MD:Z:")

		// 解析MD字符串检查错配
		mdHasDelete, mdHasSubstitution := checkMismatchInMD(mdStr)
		if mdHasDelete {
			hasDelete = true
		}
		if mdHasSubstitution {
			hasSubstitution = true
		}
	}

	// 3. 根据组合确定类型
	if !hasInsert && !hasDelete && !hasSubstitution {
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

	return ReadTypeMatch
}

// analyzeReadDetailedInfo 分析详细的read信息
func analyzeReadDetailedInfo(read *sam.Record, mdMap map[int]string, mdStr, refSeq string) ReadDetailedInfo {
	var info ReadDetailedInfo

	// 基本类型分析
	info.MainType = analyzeReadType(read)

	// 分析插入子类型
	if hasInsertInCigar(read) {
		info.InsertSub = analyzeInsertSubtype(read, refSeq)
	}

	// 分析缺失子类型
	if hasDeleteInCigar(read) {
		info.DeleteSub = analyzeDeleteSubtype(read, mdStr)
	}

	// 解析突变
	info.Mutations = parseMutationsWithMD(read, refSeq, mdMap)
	return info
}

// hasInsertInCigar 检查CIGAR中是否有插入
func hasInsertInCigar(read *sam.Record) bool {
	for _, cigarOp := range read.Cigar {
		if cigarOp.Type() == 1 { // I: 插入
			return true
		}
	}
	return false
}

// hasDeleteInCigar 检查CIGAR中是否有缺失
func hasDeleteInCigar(read *sam.Record) bool {
	for _, cigarOp := range read.Cigar {
		if cigarOp.Type() == 2 || cigarOp.Type() == 3 { // D, N: 缺失
			return true
		}
	}
	return false
}

// analyzeInsertSubtype 分析插入子类型，使用参考序列
func analyzeInsertSubtype(read *sam.Record, refSeq string) *InsertSubtype {
	subtype := &InsertSubtype{}
	seq := read.Seq.Expand()
	refPos := int(read.Pos) // 0-basedd
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == sam.CigarInsertion { // I: 插入
		}

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
			readPos += length
		case sam.CigarInsertion: // I
			insertionSubtype := classifyInsertion(seq, readPos, length, refSeq, refPos)
			// DupDup 转换为 2个Dup1
			if insertionSubtype == DupDup {
				insertion1 := InsertionInfo{
					Bases:   string(seq[readPos:min(readPos+1, len(seq))]),
					Length:  1,
					Subtype: Dup1,
				}
				insertion2 := InsertionInfo{
					Bases:   string(seq[readPos+1 : min(readPos+2, len(seq))]),
					Length:  1,
					Subtype: Dup1,
				}
				subtype.Insertions = append(subtype.Insertions, insertion1, insertion2)
			} else {
				insertion := InsertionInfo{
					Bases:   string(seq[readPos:min(readPos+length, len(seq))]),
					Length:  length,
					Subtype: insertionSubtype,
				}
				subtype.Insertions = append(subtype.Insertions, insertion)
			}
			readPos += length
		case sam.CigarSoftClipped: // I, S
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
		deletedInfos := parseDeletionInfoFromMD(mdStr, refStart)
		for i := range deletedInfos {
			deletedInfos[i].Subtype = classifyDeletion(deletedInfos[i].Length)
		}
		subtype.Deletions = deletedInfos
		return subtype
	}

	// 如果没有MD标签，只从CIGAR获取长度和估算位置
	refPos := refStart
	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == sam.CigarDeletion || op == sam.CigarSkipped { // D, N: 缺失
			// 记录缺失长度
			deletion := DeletionInfo{
				Length:   length,
				Position: refPos + 1, // 转换为1-based
				Subtype:  classifyDeletion(length),
			}
			subtype.Deletions = append(subtype.Deletions, deletion)
		}

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped: // I, S
			// 插入，不增加参考位置
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			refPos += length
		}
	}
	return subtype
}

// 新增：生成read的细分类组合键
func buildSubtypeCombinationKey(insertSubtypes map[InsertionSubtype]bool, deleteSubtypes map[DeletionSubtype]bool, substSubtypes map[SubstitutionSubtype]bool) string {
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

// parseDeletionInfoFromMD 从MD字符串解析缺失信息（包括位置）
func parseDeletionInfoFromMD(mdStr string, refStart int) []DeletionInfo {
	var deletions []DeletionInfo

	i := 0
	refPos := refStart // 0-based位置

	for i < len(mdStr) {
		if isDigit(mdStr[i]) {
			// 读取数字（匹配长度）
			j := i
			for j < len(mdStr) && isDigit(mdStr[j]) {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				i = j
				continue
			}

			// 匹配区域，增加参考位置
			refPos += num
			i = j
		} else if mdStr[i] == '^' {
			// 删除标记开始
			i++
			start := i
			// 收集删除的碱基
			for i < len(mdStr) && !isDigit(mdStr[i]) {
				i++
			}

			if start < i {
				bases := mdStr[start:i]
				deletion := DeletionInfo{
					Length:   len(bases),
					Bases:    bases,
					Position: refPos + 1, // 转换为1-based
				}
				deletions = append(deletions, deletion)

				// 删除区域也增加参考位置
				refPos += len(bases)
			}
		} else if isBase(mdStr[i]) {
			// 错配碱基，增加参考位置
			refPos++
			// 错配碱基，跳过
			i++
		} else {
			i++
		}
	}

	return deletions
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// classifyInsertion 判定插入细分类
// 依赖 参考序列上下文（-1/+1位置）
func classifyInsertion(seq []byte, readPos int, length int, refSeq string, refPos int) InsertionSubtype {
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

// 新增：缺失细分类判定
func classifyDeletion(length int) DeletionSubtype {
	switch length {
	case 1:
		return Del1
	case 2:
		return Del2
	default: // >2
		return Del3
	}
}
