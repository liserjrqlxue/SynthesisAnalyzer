package main

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// readExcelSampleOrder 使用 excelize 读取 Excel 文件，基于表头确定列
// 返回: 样品顺序列表, 样品名->全长参考序列, 样品名->头切除长度, 样品名->尾切除长度, 错误
func readExcelSampleOrder(filePath string) ([]string, map[string]string, map[string]int, map[string]int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("打开Excel文件失败: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("读取Sheet1失败: %v", err)
	}
	if len(rows) < 2 {
		return nil, nil, nil, nil, fmt.Errorf("Excel文件至少需要表头和一行数据")
	}

	// 解析表头，找到各列索引
	header := rows[0]
	nameCol := -1
	targetCol := -1
	synthCol := -1
	postCol := -1

	for i, colName := range header {
		trimmed := strings.TrimSpace(colName)
		switch trimmed {
		case "样品名称":
			nameCol = i
		case "靶标序列":
			targetCol = i
		case "合成序列":
			synthCol = i
		case "后靶标":
			postCol = i
		}
	}

	// 检查必需列
	if nameCol == -1 {
		return nil, nil, nil, nil, fmt.Errorf("未找到'样品名称'列")
	}
	if targetCol == -1 {
		return nil, nil, nil, nil, fmt.Errorf("未找到'靶标序列'列")
	}
	if synthCol == -1 {
		return nil, nil, nil, nil, fmt.Errorf("未找到'合成序列'列")
	}
	if postCol == -1 {
		return nil, nil, nil, nil, fmt.Errorf("未找到'后靶标'列")
	}

	var samples []string
	fullSeqs := make(map[string]string)
	headCuts := make(map[string]int)
	tailCuts := make(map[string]int)

	// 遍历数据行（从第二行开始）
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		// 确保行长度足够
		if len(row) <= nameCol || len(row) <= targetCol || len(row) <= synthCol || len(row) <= postCol {
			fmt.Printf("警告: 第 %d 行缺少必要的列数据，跳过\n", i+1)
			continue
		}
		sampleName := strings.TrimSpace(row[nameCol])
		targetSeq := strings.TrimSpace(row[targetCol])
		synthSeq := strings.TrimSpace(row[synthCol])
		postSeq := strings.TrimSpace(row[postCol])

		if sampleName == "" {
			fmt.Printf("警告: 第 %d 行样品名称为空，跳过\n", i+1)
			continue
		}

		fullSeq := targetSeq + synthSeq + postSeq
		fullSeqs[sampleName] = fullSeq
		headCuts[sampleName] = len(targetSeq) // 头切除长度 = 靶标序列长度
		tailCuts[sampleName] = len(postSeq)   // 尾切除长度 = 后靶标长度
		samples = append(samples, sampleName)
	}

	if len(samples) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("没有有效的数据行")
	}

	return samples, fullSeqs, headCuts, tailCuts, nil
}
