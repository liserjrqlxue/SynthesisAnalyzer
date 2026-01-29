package main

import "github.com/tealeg/xlsx"

func readExcelSampleOrder(filePath string) ([]string, error) {
	xlFile, err := xlsx.OpenFile(filePath)
	if err != nil {
		return nil, err
	}

	var samples []string
	var cIdx = -1
	// 假设样本名称在Sheet1的第一列
	sheet := xlFile.Sheets[0]
	for i, row := range sheet.Rows {
		if i == 0 {
			// 跳过表头
			for j := range len(row.Cells) {
				if row.Cells[j].String() == "样品名称" {
					cIdx = j
					break
				}

			}
			continue
		}
		if len(row.Cells) > cIdx {
			sampleName := row.Cells[cIdx].String()
			if sampleName != "" {
				samples = append(samples, sampleName)
			}
		}
	}

	return samples, nil
}
