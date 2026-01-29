package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// readExcelSampleOrder 读取Excel文件中的样本顺序
func readExcelSampleOrder(filePath string) ([]string, error) {
	// 这里我们假设Excel是CSV格式，第一列是"样品名称"
	// 如果需要读取真正的Excel文件，可以使用github.com/tealeg/xlsx等库
	// 这里我们简化处理，假设Excel是CSV格式
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开Excel文件失败: %v", err)
	}
	defer f.Close()

	var samples []string
	scanner := bufio.NewScanner(f)
	headerRead := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if !headerRead {
			// 跳过表头
			if strings.Contains(line, "样品名称") {
				headerRead = true
			}
			continue
		}

		// 假设列之间用逗号分隔
		fields := strings.Split(line, ",")
		if len(fields) > 0 {
			sampleName := strings.TrimSpace(fields[0])
			if sampleName != "" {
				samples = append(samples, sampleName)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取Excel文件失败: %v", err)
	}

	return samples, nil
}
