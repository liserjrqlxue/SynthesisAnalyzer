package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/biogo/hts/sam"
)

// parseMutationsWithMD 使用MD标签获取所有突变（包括CIGAR X和MD中的错配）
func parseMutationsWithMD(read *sam.Record) []Mutation {
	var mutations []Mutation

	refStart := int(read.Pos) // 0-based起始位置
	seq := read.Seq.Expand()

	if len(seq) == 0 {
		return mutations
	}

	// 获取MD标签
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	mdStr := ""
	if hasMD {
		mdStr = mdTag.String()
		mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
	}

	// 解析MD字符串，建立位置到参考碱基的映射
	mdMap := parseMDToMap(mdStr, refStart)

	// 遍历CIGAR操作
	refPos := refStart // 0-based
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		switch op {
		case sam.CigarMismatch:
			// X操作 - 直接作为替换
			for i := range length {
				position := refPos + i + 1 // 转换为1-based输出

				// 从MD映射获取参考碱基
				refBase, ok := mdMap[refPos+i] // 使用0-based位置查询
				if !ok {
					// 如果MD映射中没有，尝试从上下文推断
					refBase = inferRefBaseFromMD(mdStr, (refPos+i)-refStart)
				}

				// 获取read碱基作为替代碱基
				altBase := "N"
				if readPos+i < len(seq) {
					altBase = string(seq[readPos+i])
				}

				mutations = append(mutations, Mutation{
					Position: position,
					Ref:      refBase,
					Alt:      altBase,
				})
			}
		// case sam.CigarMatch, sam.CigarEqual: // M或=操作 - 需要从MD中检查错配
		case sam.CigarMatch: // M操作 - 需要从MD中检查错配
			// 对于M操作，我们需要检查MD中是否有错配
			for i := range length {
				position := refPos + i + 1 // 转换为1-based输出

				// 检查这个位置在MD中是否标记为错配
				if refBase, ok := mdMap[refPos+i]; ok {
					// MD中有这个位置，说明是错配
					// 获取read碱基作为替代碱基
					altBase := "N"
					if readPos+i < len(seq) {
						altBase = string(seq[readPos+i])
					}

					if refBase != altBase {
						mutations = append(mutations, Mutation{
							Position: position,
							Ref:      refBase,
							Alt:      altBase,
						})
					}
				}
			}
		}

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
			readPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped: // I, S
			readPos += length
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			refPos += length
		}
	}

	return mutations
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

// inferRefBaseFromMD 从MD字符串推断参考碱基
func inferRefBaseFromMD(mdStr string, relPos int) string {
	// 简化实现：遍历MD字符串
	pos := 0
	i := 0

	for i < len(mdStr) {
		if isDigit(mdStr[i]) {
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

			if relPos >= pos && relPos < pos+num {
				// 在匹配区域，参考碱基未知
				return "N"
			}

			pos += num
			i = j
		} else if isBase(mdStr[i]) {
			if pos == relPos {
				return string(mdStr[i])
			}
			pos++
			i++
		} else if mdStr[i] == '^' {
			i++
			for i < len(mdStr) && isBase(mdStr[i]) {
				if pos == relPos {
					return string(mdStr[i])
				}
				pos++
				i++
			}
		} else {
			i++
		}
	}

	return "N"
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

// analyzeDetailedReadTypeWithMD 分析详细的read类型（带MD字符串）
func analyzeDetailedReadTypeWithMD(read *sam.Record, mdStr string) ReadDetailedType {
	var result ReadDetailedType

	// 基本类型分析
	result.MainType = analyzeReadType(read)

	// 检查是否有突变
	result.HasMutation = (result.MainType != ReadTypeMatch)

	// 获取参考起始位置
	refStart := int(read.Pos) // 0-based

	// 分析插入子类型
	if hasInsertInCigar(read) {
		result.InsertSub = analyzeInsertSubtype(read)
	}

	// 分析缺失子类型
	if hasDeleteInCigar(read) {
		result.DeleteSub = analyzeDeleteSubtype(read, mdStr, refStart)
	}

	return result
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

// analyzeInsertSubtype 分析插入子类型
func analyzeInsertSubtype(read *sam.Record) *InsertSubtype {
	subtype := &InsertSubtype{}

	seq := read.Seq.Expand()
	refStart := int(read.Pos) // 0-basedd

	// 分析CIGAR操作
	refPos := refStart
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == sam.CigarInsertion { // I: 插入
			// 记录插入长度
			insertion := InsertionInfo{
				Length: length,
			}
			// 获取插入序列
			if readPos+length <= len(seq) {
				insertion.Bases = string(seq[readPos : readPos+length])
			}

			// 检查插入是否与两边碱基相同
			if readPos > 0 && readPos+length < len(seq) && len(insertion.Bases) > 0 {
				leftBase := seq[readPos-1]
				rightBase := seq[readPos+length]

				// 检查插入序列是否都与左边或右边碱基相同
				allSameAsLeft := true
				allSameAsRight := true
				for i := 0; i < len(insertion.Bases); i++ {
					if insertion.Bases[i] != leftBase {
						allSameAsLeft = false
					}
					if insertion.Bases[i] != rightBase {
						allSameAsRight = false
					}
				}

				insertion.IsSameAsFlanking = allSameAsLeft || allSameAsRight
			}
			subtype.Insertions = append(subtype.Insertions, insertion)
		}

		// 更新位置
		switch op {
		case sam.CigarMatch, sam.CigarEqual, sam.CigarMismatch: // M, =, X
			refPos += length
			readPos += length
		case sam.CigarInsertion, sam.CigarSoftClipped: // I, S
			readPos += length
		case sam.CigarDeletion, sam.CigarSkipped: // D, N
			refPos += length
		}
	}

	return subtype
}

// analyzeDeleteSubtype 分析缺失子类型 - 增加位置信息
func analyzeDeleteSubtype(read *sam.Record, mdStr string, refStart int) *DeleteSubtype {
	subtype := &DeleteSubtype{}

	// 从MD字符串解析缺失的碱基和位置
	if mdStr != "" {
		// 解析MD字符串获取缺失的碱基和位置
		deletedInfos := parseDeletionInfoFromMD(mdStr, refStart)
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

// getDetailedTypeKey 获取详细类型的字符串键
func getDetailedTypeKey(detailedType ReadDetailedType) string {
	key := ReadTypeNames[detailedType.MainType]

	// 添加插入子类型信息
	if detailedType.InsertSub != nil && len(detailedType.InsertSub.Insertions) > 0 {
		// 统计各长度插入的个数
		len1, len2, len3, lenMore := 0, 0, 0, 0
		sameCount, diffCount := 0, 0

		for _, insertion := range detailedType.InsertSub.Insertions {
			switch {
			case insertion.Length == 1:
				len1++
			case insertion.Length == 2:
				len2++
			case insertion.Length == 3:
				len3++
			default:
				lenMore++
			}

			if insertion.IsSameAsFlanking {
				sameCount++
			} else {
				diffCount++
			}
		}

		// 添加插入长度分布
		if len1 > 0 {
			key += fmt.Sprintf("_I1x%d", len1)
		}
		if len2 > 0 {
			key += fmt.Sprintf("_I2x%d", len2)
		}
		if len3 > 0 {
			key += fmt.Sprintf("_I3x%d", len3)
		}
		if lenMore > 0 {
			key += fmt.Sprintf("_I>3x%d", lenMore)
		}

		// 添加是否与两边相同
		if sameCount > 0 {
			key += fmt.Sprintf("_same%d", sameCount)
		}
		if diffCount > 0 {
			key += fmt.Sprintf("_diff%d", diffCount)
		}
	}

	// 添加缺失子类型信息
	if detailedType.DeleteSub != nil && len(detailedType.DeleteSub.Deletions) > 0 {
		// 统计各长度缺失的个数
		len1, len2, len3, lenMore := 0, 0, 0, 0
		maxLen := 0

		for _, deletion := range detailedType.DeleteSub.Deletions {
			switch {
			case deletion.Length == 1:
				len1++
			case deletion.Length == 2:
				len2++
			case deletion.Length == 3:
				len3++
			default:
				lenMore++
			}

			if deletion.Length > maxLen {
				maxLen = deletion.Length
			}
		}

		// 添加缺失长度分布
		if len1 > 0 {
			key += fmt.Sprintf("_D1x%d", len1)
		}
		if len2 > 0 {
			key += fmt.Sprintf("_D2x%d", len2)
		}
		if len3 > 0 {
			key += fmt.Sprintf("_D3x%d", len3)
		}
		if lenMore > 0 {
			key += fmt.Sprintf("_D>3x%d", lenMore)
		}

		// 添加最长缺失长度
		maxLenLabel := ""
		switch {
		case maxLen == 1:
			maxLenLabel = "_maxD1"
		case maxLen == 2:
			maxLenLabel = "_maxD2"
		case maxLen == 3:
			maxLenLabel = "_maxD3"
		case maxLen > 3:
			maxLenLabel = "_maxD>3"
		}
		key += maxLenLabel
	}

	return key
}

// updateBaseMutationCounts 更新碱基维度的变异计数 - 增加位置统计
func updateBaseMutationCounts(read *sam.Record, mutations []Mutation, insertSub *InsertSubtype, deleteSub *DeleteSubtype, stats *MutationStats, sampleName string) {
	stats.Lock()
	defer stats.Unlock()

	// 初始化样本的计数
	if _, ok := stats.MutationBaseCounts[sampleName]; !ok {
		stats.MutationBaseCounts[sampleName] = make(map[string]int)
	}
	if _, ok := stats.SampleDeleteBaseCounts[sampleName]; !ok {
		stats.SampleDeleteBaseCounts[sampleName] = make(map[byte]int)
	}
	if _, ok := stats.SampleInsertBaseCounts[sampleName]; !ok {
		stats.SampleInsertBaseCounts[sampleName] = make(map[string]int)
	}
	if _, ok := stats.SampleDeletePositionCounts[sampleName]; !ok {
		stats.SampleDeletePositionCounts[sampleName] = make(map[string]int)
	}

	// 1. 统计替换（碱基维度）
	stats.MutationBaseCounts[sampleName]["substitution"] += len(mutations)

	// 2. 统计插入（碱基维度）
	if insertSub != nil {
		for _, insertion := range insertSub.Insertions {
			// 插入碱基数
			stats.MutationBaseCounts[sampleName]["insertion"] += insertion.Length

			// 统计插入序列
			if insertion.Bases != "" {
				stats.SampleInsertBaseCounts[sampleName][insertion.Bases]++
				stats.TotalInsertBaseCounts[insertion.Bases]++
			}
		}
	}

	// 3. 统计缺失（碱基维度）
	if deleteSub != nil {
		for _, deletion := range deleteSub.Deletions {
			// 缺失碱基数
			stats.MutationBaseCounts[sampleName]["deletion"] += deletion.Length

			// 统计单碱基缺失的碱基类型
			if deletion.Length == 1 && len(deletion.Bases) == 1 {
				base := deletion.Bases[0]
				stats.SampleDeleteBaseCounts[sampleName][base]++
				stats.TotalDeleteBaseCounts[base]++

				// 记录位置信息
				posKey := fmt.Sprintf("%d:%c", deletion.Position, base)
				stats.SampleDeletePositionCounts[sampleName][posKey]++
				stats.TotalDeletePositionCounts[posKey]++
			}
		}
	}
}
