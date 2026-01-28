package main

import (
	"strconv"
	"strings"

	"github.com/biogo/hts/sam"
)

// parseCigarXWithMD 使用MD标签获取参考碱基的X操作解析
func parseCigarXWithMD(read *sam.Record) []Mutation {
	var mutations []Mutation

	refStart := int(read.Pos)
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
	refPos := refStart - 1 // 0-based
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == 8 { // X操作
			for i := 0; i < length; i++ {
				position := refPos + i + 1 // 1-based

				// 从MD映射获取参考碱基
				refBase, ok := mdMap[position]
				if !ok {
					// 如果MD映射中没有，尝试其他方法
					refBase = inferRefBase(mdStr, position-refStart+1)
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
		case 0, 7, 8:
			refPos += length
			readPos += length
		case 1, 4:
			readPos += length
		case 2, 3:
			refPos += length
		}
	}

	return mutations
}

// parseMDToMap 解析MD字符串，返回位置->参考碱基的映射
func parseMDToMap(mdStr string, refStart int) map[int]string {
	result := make(map[int]string)

	if mdStr == "" {
		return result
	}

	pos := refStart // 当前位置（1-based）
	i := 0

	for i < len(mdStr) {
		// 读取数字
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

			pos += num
			i = j
		} else if isBase(mdStr[i]) {
			// 错配
			result[pos] = string(mdStr[i])
			pos++
			i++
		} else if mdStr[i] == '^' {
			// 删除
			i++
			for i < len(mdStr) && isBase(mdStr[i]) {
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

// inferRefBase 从MD字符串推断参考碱基
func inferRefBase(mdStr string, relPos int) string {
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

			if relPos > pos && relPos <= pos+num {
				// 在匹配区域，但我们不知道具体碱基
				return "N"
			}

			pos += num
			i = j
		} else if isBase(mdStr[i]) {
			pos++
			if pos == relPos {
				return string(mdStr[i])
			}
			i++
		} else if mdStr[i] == '^' {
			i++
			for i < len(mdStr) && isBase(mdStr[i]) {
				pos++
				if pos == relPos {
					return string(mdStr[i])
				}
				i++
			}
		} else {
			i++
		}
	}

	return "N"
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
