package main

import "github.com/tealeg/xlsx"

func readExcelSampleOrder(filePath string) ([]string, error) {
	xlFile, err := xlsx.OpenFile(filePath)
	if err != nil {
		return nil, err
	}

	var samples []string
	// 假设样本名称在Sheet1的第一列
	sheet := xlFile.Sheets[0]
	for i, row := range sheet.Rows {
		if i == 0 {
			// 跳过表头
			continue
		}
		if len(row.Cells) > 0 {
			sampleName := row.Cells[0].String()
			if sampleName != "" {
				samples = append(samples, sampleName)
			}
		}
	}

	return samples, nil
}
