package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func main() {
	var (
		inputDir   = flag.String("i", "", "输入目录，包含多个TXT文件（必填）")
		outputFile = flag.String("o", "", "输出Excel文件路径（默认: 目录名.xlsx）")
	)
	flag.Parse()

	if *inputDir == "" {
		log.Fatal("请使用 -i 指定输入目录")
	}

	// 如果使用默认输出文件名，基于输入目录的basename
	if *outputFile == "" {
		absDir, err := filepath.Abs(*inputDir)
		if err != nil {
			log.Fatalf("获取绝对路径失败: %v", err)
		}
		dirName := filepath.Base(absDir)
		*outputFile = dirName + ".xlsx"
		log.Printf("使用默认输出文件名: %s", *outputFile)
	}

	// 查找所有txt文件
	txtFiles, err := findTXTFiles(*inputDir)
	if err != nil {
		log.Fatalf("查找TXT文件失败: %v", err)
	}

	if len(txtFiles) == 0 {
		log.Fatal("未找到任何TXT文件")
	}

	log.Printf("找到 %d 个TXT文件", len(txtFiles))

	// 解析所有TXT文件
	allData := make(map[string]map[string]string) // filename -> key -> value
	allBatchIDs := make(map[string]string)        // filename -> batchID
	validFiles := make([]string, 0)               // 保持输入顺序的有效文件列表
	keyOrder := make([]string, 0)                 // 第一个文件的key顺序
	hasFirstFile := false

	for _, txtFile := range txtFiles {
		data, batchID, err := parseTXTFile(txtFile)
		if err != nil {
			log.Printf("解析文件 %s 失败: %v，跳过", txtFile, err)
			continue
		}
		baseName := filepath.Base(txtFile)
		allData[baseName] = data
		allBatchIDs[baseName] = batchID
		validFiles = append(validFiles, baseName) // 保持输入顺序

		// 从第一个文件记录key顺序
		if !hasFirstFile {
			// 重新扫描文件获取key的顺序
			file, err := os.Open(txtFile)
			if err == nil {
				scanner := bufio.NewScanner(file)
				inDataSection := false
				hasValidHeader := false
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())

					// 检查文件头
					if !hasValidHeader {
						if strings.HasPrefix(line, "合成下机报告:") {
							hasValidHeader = true
						}
						continue
					}

					// 检测章节
					if line == "基本信息" || line == "合成错误统计" {
						inDataSection = true
						continue
					}

					// 解析Tab分割的数据
					if inDataSection {
						parts := strings.Split(line, "\t")
						if len(parts) >= 2 {
							key := strings.TrimSpace(parts[0])
							if key != "" && key != "错误类型-CN" {
								keyOrder = append(keyOrder, key)
							}
						}
					}
				}
				file.Close()
			}
			hasFirstFile = true
		} else {
			// 验证后续文件是否包含所有必要的key
			for _, key := range keyOrder {
				if _, exists := data[key]; !exists {
					log.Printf("警告：文件 %s 缺少key: %s", txtFile, key)
				}
			}
		}
	}

	if len(validFiles) == 0 {
		log.Fatal("未找到任何有效的TXT文件")
	}

	log.Printf("成功解析 %d 个TXT文件", len(validFiles))

	// 生成Excel
	if err := generateExcel(*outputFile, keyOrder, allData, allBatchIDs, validFiles); err != nil {
		log.Fatalf("生成Excel失败: %v", err)
	}

	log.Printf("Excel文件已生成: %s", *outputFile)
}

// findTXTFiles 查找目录下所有txt文件（保持文件顺序）
func findTXTFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var txtFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			txtFiles = append(txtFiles, filepath.Join(dir, entry.Name()))
		}
	}

	return txtFiles, nil
}

// parseTXTFile 解析TXT文件，提取key-value对和BatchID
func parseTXTFile(filename string) (map[string]string, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	data := make(map[string]string)
	batchID := ""
	scanner := bufio.NewScanner(file)
	inDataSection := false
	hasValidHeader := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 检查文件是否以"合成下机报告:"开头
		if !hasValidHeader {
			if strings.HasPrefix(line, "合成下机报告:") {
				hasValidHeader = true
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					batchID = strings.TrimSpace(parts[1])
				}
				continue
			} else if line != "" {
				// 文件开头不是"合成下机报告:"，不是有效的报告文件
				return nil, "", fmt.Errorf("不是有效的合成下机报告文件")
			}
			continue
		}

		// 提取BatchID（如果前面没提取到）
		if batchID == "" && strings.HasPrefix(line, "合成下机报告:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				batchID = strings.TrimSpace(parts[1])
			}
			continue
		}

		// 跳过空行和分隔线
		if line == "" || strings.HasPrefix(line, "=") || strings.HasPrefix(line, "-") {
			continue
		}

		// 检测章节开始
		if line == "基本信息" || line == "合成错误统计" {
			inDataSection = true
			continue
		}

		// 解析Tab分割的数据（统一格式）
		if inDataSection {
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// 跳过表头行
				if key != "" && key != "错误类型-CN" {
					data[key] = value
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, "", err
	}

	return data, batchID, nil
}

// generateExcel 生成Excel文件
func generateExcel(outputFile string, keyOrder []string, allData map[string]map[string]string, allBatchIDs map[string]string, fileNames []string) error {
	f := excelize.NewFile()
	sheetName := "合并数据"
	f.SetSheetName("Sheet1", sheetName)

	// 写入表头
	// 第一行：合成下机报告 BatchID1 BatchID2 BatchID3
	f.SetCellValue(sheetName, "A1", "合成下机报告")
	for i, fileName := range fileNames {
		col := getColumnName(i + 2) // B列开始
		batchID := allBatchIDs[fileName]
		if batchID == "" {
			batchID = fileName // 如果没有BatchID，使用文件名
		}
		f.SetCellValue(sheetName, col+"1", batchID)
	}

	// 写入数据
	for rowIdx, key := range keyOrder {
		rowNum := rowIdx + 2 // 从第2行开始
		// 第一列是说明
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), key)

		// 后面每列是对应的值
		for colIdx, fileName := range fileNames {
			col := getColumnName(colIdx + 2)
			value := allData[fileName][key]
			f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, rowNum), value)
		}
	}

	// 设置列宽
	f.SetColWidth(sheetName, "A", "A", 30)
	for i := range fileNames {
		col := getColumnName(i + 2)
		f.SetColWidth(sheetName, col, col, 25)
	}

	// 保存文件
	if err := f.SaveAs(outputFile); err != nil {
		return err
	}

	return nil
}

// getColumnName 将列索引转换为Excel列名 (1 -> A, 2 -> B, 27 -> AA)
func getColumnName(index int) string {
	result := ""
	for index > 0 {
		index--
		result = string(rune('A'+index%26)) + result
		index /= 26
	}
	return result
}
