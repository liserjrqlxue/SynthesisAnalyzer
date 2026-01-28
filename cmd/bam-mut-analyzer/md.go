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

/*
// parseCigarXUsingSAM 使用sam包的内置功能解析CIGAR X操作
func parseCigarXUsingSAM(read *sam.Record) []Mutation {
	var mutations []Mutation

	// 检查是否有MD标签
	mdTag, hasMD := read.Tag([]byte{'M', 'D'})
	if !hasMD {
		return mutations
	}

	// 使用sam.NewMD方法解析MD标签
	md, err := sam.NewMD(mdTag.String())
	if err != nil {
		return mutations
	}

	refStart := int(read.Pos) // 0-based起始位置
	seq := read.Seq.Expand()

	if len(seq) == 0 {
		return mutations
	}

	// 遍历CIGAR操作
	refPos := refStart // 0-based
	readPos := 0

	for _, cigarOp := range read.Cigar {
		op := cigarOp.Type()
		length := cigarOp.Len()

		if op == 8 { // X操作
			for i := 0; i < length; i++ {
				position := refPos + i + 1 // 转换为1-based输出

				// 尝试从MD标签获取参考碱基
				refBase := getRefBaseFromSAMMD(md, (refPos+i)-refStart)

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

// getRefBaseFromSAMMD 从sam.MD对象获取参考碱基
func getRefBaseFromSAMMD(md *sam.MD, pos int) string {
	// sam.MD类型没有直接的方法获取特定位置的碱基
	// 我们需要遍历MD操作
	mdPos := 0

	for _, op := range md.Ops {
		switch op.Type {
		case 0: // 匹配
			if pos >= mdPos && pos < mdPos+op.Len {
				// 在匹配区域，参考碱基未知
				return "N"
			}
			mdPos += op.Len
		case 1: // 错配
			if mdPos == pos {
				return string(op.Base)
			}
			mdPos++
		case 2: // 删除
			for _, base := range op.Seq {
				if mdPos == pos {
					return string(base)
				}
				mdPos++
			}
		}
	}

	return "N"
}
*/

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
