package cfg

import (
	"log"
	"strconv"
)

// CheckMismatchInMD 检查MD字符串中是否有错配和删除
func CheckMismatchInMD(mdStr string) (bool, bool) {
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

// ParseMDToMap 解析MD字符串，返回位置(0-based)->参考碱基的映射（只包含错配）
func ParseMDToMap(mdStr string, refStart int) map[int]string {
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

// ParseDeletionInfoFromMD 从MD字符串解析缺失信息（包括位置）
func ParseDeletionInfoFromMD(mdStr string, refStart int) []DeletionInfo {
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
