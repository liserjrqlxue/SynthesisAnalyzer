package main

import (
	"strconv"
	"strings"

	"github.com/biogo/hts/sam"
)

// parseCigarXWithMD 使用MD标签获取参考碱基的X操作解析
func parseCigarXWithMD(read *sam.Record) []Mutation {
	var mutations []Mutation

	refStart := int(read.Pos) // 0-based起始位置
	seq := read.Seq.Expand()

	if len(seq) == 0 {
		return mutations
	}

	// 获取MD标签
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	if !hasMD {
		return mutations
	}

	mdStr := mdTag.String()
	mdStr = strings.TrimPrefix(mdStr, "MD:Z:")

	// 解析MD字符串，建立位置到参考碱基的映射
	mdMap := parseMDToMap(mdStr, refStart)

	// 遍历CIGAR操作
	refPos := refStart // 0-based
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == 8 { // X操作
			for i := 0; i < length; i++ {
				position := refPos + i + 1 // 转换为1-based输出

				// 从MD映射获取参考碱基
				refBase, ok := mdMap[refPos+i] // 使用0-based位置查询
				if !ok {
					// 如果MD映射中没有，尝试从上下文推断
					refBase = inferRefBaseFromMD(mdStr, (refPos+i)-refStart)
				}

				// 获取read碱基
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
		}

		// 更新位置
		switch op {
		case 0, 7, 8: // M, =, X
			refPos += length
			readPos += length
		case 1, 4: // I, S
			readPos += length
		case 2, 3: // D, N
			refPos += length
		}
	}

	return mutations
}

// parseMDToMap 解析MD字符串，返回位置(0-based)->参考碱基的映射
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
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
				j++
			}
			numStr := mdStr[i:j]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				i = j
				continue
			}

			// 匹配区域，不记录到映射中
			pos += num
			i = j
		} else if isBase(mdStr[i]) {
			// 错配（碱基）
			result[pos] = string(mdStr[i])
			pos++
			i++
		} else if mdStr[i] == '^' {
			// 删除
			i++
			for i < len(mdStr) && isBase(mdStr[i]) {
				// 删除的碱基记录到映射中
				result[pos] = string(mdStr[i])
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
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			j := i
			for j < len(mdStr) && mdStr[j] >= '0' && mdStr[j] <= '9' {
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

// countXOperations 统计CIGAR中的X操作数
func countXOperations(read *sam.Record) int {
	count := 0
	for _, cigarOp := range read.Cigar {
		if cigarOp.Type() == 8 { // X操作
			count += cigarOp.Len()
		}
	}
	return count
}

// checkMismatchInMD 检查MD字符串中是否有错配和删除 - 简化版本
func checkMismatchInMD(mdStr string) (bool, bool) {
	hasDelete := false
	hasSubstitution := false

	i := 0
	for i < len(mdStr) {
		if mdStr[i] >= '0' && mdStr[i] <= '9' {
			// 跳过数字
			i++
			for i < len(mdStr) && mdStr[i] >= '0' && mdStr[i] <= '9' {
				i++
			}
		} else if mdStr[i] == '^' {
			hasDelete = true
			i++
			// 跳过所有非数字字符（删除的碱基）
			for i < len(mdStr) && !(mdStr[i] >= '0' && mdStr[i] <= '9') {
				i++
			}
		} else {
			// 不是数字也不是'^'，就是错配碱基
			hasSubstitution = true
			i++
		}
	}

	return hasDelete, hasSubstitution
}

// analyzeReadType 分析read的类型 - 修正版本
func analyzeReadType(read *sam.Record) ReadType {
	hasInsert := false
	hasDelete := false
	hasSubstitution := false

	// 检查CIGAR操作
	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()

		switch op {
		case 1: // I: 插入
			hasInsert = true
		case 2, 3: // D, N: 缺失或跳过
			hasDelete = true
		case 8: // X: 替换
			hasSubstitution = true
		}
	}

	// 检查是否有MD标签
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	if hasMD {
		mdStr := mdTag.String()
		mdStr = strings.TrimPrefix(mdStr, "MD:Z:")

		// 从MD字符串检查删除和错配
		mdHasDelete, mdHasSubstitution := checkMismatchInMD(mdStr)
		if mdHasDelete {
			hasDelete = true
		}
		if mdHasSubstitution {
			hasSubstitution = true
		}
	}

	// 根据组合确定类型
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
