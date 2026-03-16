package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 子类型分类映射（中文名称）
var subtypeMapping = map[string]string{
	"Del1":          "缺失1",
	"Del2":          "连续两个缺失",
	"Del3":          "连续三个及以上缺失",
	"MismatchA>G":   "A-G突变",
	"MismatchC>T":   "C-T突变",
	"OtherMismatch": "其余突变",
	"Dup1":          "重复插入1个",
	"Dup2":          "重复插入2个",
	"Ins1":          "插入1（与前后不同）",
	"Ins2":          "插入2（与前后不同）",
	"Ins3":          "插入3个及以上（与前后不同）",
	"DelDup":        "缺失+重复",
	"DupDel":        "重复+缺失",
}

// ErrorTypeInfo 错误类型详细信息
type ErrorTypeInfo struct {
	EN      string // 英文名称
	CN      string // 中文名称
	Example string // 示例
}

// 错误类型详细信息映射
var errorTypeInfoMap = map[string]ErrorTypeInfo{
	"Del1":          {EN: "DEL1", CN: "缺失1", Example: "ATG>AG"},
	"Del2":          {EN: "DEL2", CN: "连续两个缺失", Example: "ATCG>AG"},
	"Del3":          {EN: "DEL3", CN: "连续三个及以上缺失", Example: "ATCGA>AA"},
	"MismatchA>G":   {EN: "Mismatch-AG", CN: "A-G突变", Example: "A>G"},
	"MismatchC>T":   {EN: "Mismatch-CT", CN: "C-T突变", Example: "C>T"},
	"OtherMismatch": {EN: "Other Mismatch", CN: "其余突变", Example: "A>T"},
	"Dup1":          {EN: "DUP1", CN: "重复插入1个", Example: "ATG>ATTG"},
	"Dup2":          {EN: "DUP2", CN: "重复插入2个", Example: "ATG>ATTTG"},
	"Ins1":          {EN: "INS1", CN: "插入1（与前后不同）", Example: "AG>ACG"},
	"Ins2":          {EN: "INS2", CN: "插入2（与前后不同）", Example: "AG>ACTG"},
	"Ins3":          {EN: "INS3", CN: "插入3个及以上（与前后不同）", Example: "AG>ACCTG"},
	"DelDup":        {EN: "DELDUP", CN: "缺失+重复", Example: "AG>GG"},
	"DupDel":        {EN: "DUPDEL", CN: "重复+缺失", Example: "AG>AA"},
}

// 错误类型顺序，按照subtypeMapping的key顺序
var errorTypes = []string{
	"Del1",
	"Del2",
	"Del3",
	"MismatchA>G",
	"MismatchC>T",
	"OtherMismatch",
	"Dup1",
	"Dup2",
	"Ins1",
	"Ins2",
	"Ins3",
	"DelDup",
	"DupDel",
}

// 格式化百分比（保留两位小数）
func fmtPercent(v float64) string {
	return fmt.Sprintf("%.2f%%", v)
}

// 格式化单元格（通用）
func formatCell(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmtPercent(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// 从孔列表构建二维索引 [行][列]*Well
func buildWellPlate(wells []*Well) [Rows][Cols]*Well {
	var plate [Rows][Cols]*Well
	for _, w := range wells {
		if w.Row < 1 || w.Row > Rows {
			continue
		}
		colIdx, ok := colIndex[w.ColLetter]
		if !ok {
			continue
		}
		plate[w.Row-1][colIdx] = w
	}
	return plate
}

// 从二维孔板提取指定字段的值，生成 WellPlate（用于打印）
func extractFieldPlate(plate [Rows][Cols]*Well, field string) [Rows][Cols]interface{} {
	var wp [Rows][Cols]interface{}
	for i := 0; i < Rows; i++ {
		for j := 0; j < Cols; j++ {
			w := plate[i][j]
			if w == nil {
				wp[i][j] = nil
				continue
			}
			switch field {
			case "name":
				wp[i][j] = w.Name
			case "predicted_yield":
				wp[i][j] = w.PredictedYield
			case "yield":
				wp[i][j] = w.Yield
			case "deletion":
				wp[i][j] = w.Deletion
			case "mutation":
				wp[i][j] = w.Mutation
			case "insertion":
				wp[i][j] = w.Insertion
			default:
				wp[i][j] = nil
			}
		}
	}
	return wp
}

// 计算均值和标准差
func meanStdDev(values []float64) (mean, stdDev float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))
	var sqSum float64
	for _, v := range values {
		sqSum += (v - mean) * (v - mean)
	}
	stdDev = math.Sqrt(sqSum / float64(len(values)))
	return
}

// 计算中位数
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// 计算四分位数（Q1）
func quartile(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)
	pos := float64(n) * 0.25
	idx := int(pos)
	if pos == float64(idx) {
		return (sorted[idx-1] + sorted[idx]) / 2
	}
	return sorted[idx]
}

// 从孔板提取非零数值列表（过滤零值？根据需求，收率可能为0，我们保留所有值）
func extractValues(plate [Rows][Cols]*Well, field string) []float64 {
	var vals []float64
	for i := 0; i < Rows; i++ {
		for j := 0; j < Cols; j++ {
			w := plate[i][j]
			if w == nil {
				continue
			}
			var v float64
			switch field {
			case "predicted_yield":
				v = w.PredictedYield
			case "yield":
				v = w.Yield
			case "deletion":
				v = w.Deletion
			case "mutation":
				v = w.Mutation
			case "insertion":
				v = w.Insertion
			default:
				continue
			}
			// 通常百分比字段会大于0，但0值也可能有意义，保留
			vals = append(vals, v)
		}
	}
	return vals
}

// 生成 batchID (合成日期+仪器号，仅保留字母数字)
func makeBatchID(date, instrument string) string {
	s := strings.ReplaceAll(date, ".", "") + instrument
	var out strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// ReadSubtypeStats 从read_type_summary.csv读取子类型统计数据
func ReadSubtypeStats(inputDir string) (map[string]float64, error) {
	csvPath := filepath.Join(inputDir, "mutation_stats", "read_type_summary.csv")
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("打开read_type_summary.csv失败: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 允许不同行有不同数量的字段
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("读取read_type_summary.csv失败: %w", err)
	}

	subtypeStats := make(map[string]float64)

	// 查找细分类统计部分
	inSubtypeSection := false

	for _, record := range records {
		// 跳过空行
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}

		// 查找细分类统计部分的开始
		if strings.Contains(strings.TrimSpace(record[0]), "细分类统计") {
			inSubtypeSection = true
			continue
		}

		// 查找细分类统计部分的结束
		if inSubtypeSection && strings.Contains(strings.TrimSpace(record[0]), "样品平均比例") {
			break
		}

		// 处理细分类统计数据
		if inSubtypeSection && len(record) >= 3 {
			subtypeFull := strings.TrimSpace(record[0])
			if subtypeFull == "" {
				continue
			}

			// 提取子类型（去掉前缀如"D:"、"I:"、"X:"）
			subtype := subtypeFull
			if strings.Contains(subtype, ":") {
				parts := strings.Split(subtype, ":")
				if len(parts) > 1 {
					subtype = parts[1]
				}
			}

			// 检查子类型是否在映射中
			if _, ok := subtypeMapping[subtype]; !ok {
				continue
			}

			// 尝试解析百分比值
			for j := 2; j < len(record); j++ {
				valueStr := strings.TrimSpace(record[j])
				if valueStr == "" || valueStr == "%" {
					continue
				}

				// 移除百分号并转换为float64
				valueStr = strings.TrimSuffix(valueStr, "%")
				value, err := strconv.ParseFloat(valueStr, 64)
				if err != nil {
					continue
				}

				subtypeStats[subtype] = value
				break // 只取第一个有效值
			}
		}
	}
	return subtypeStats, nil
}

// PositionStats 位置统计数据
type PositionStats struct {
	Pos          int     `json:"pos"`
	AvgYield     float64 `json:"avg_yield"`     // 平均合成收率: match_pure/depth
	AvgDeletion  float64 `json:"avg_deletion"`  // 平均缺失: deletion/depth
	AvgMutation  float64 `json:"avg_mutation"`  // 平均突变: (mismatch_pure+mismatch_with_ins)/depth
	AvgInsertion float64 `json:"avg_insertion"` // 平均插入: insertion/depth
}

// ReadPositionStats 从{ID}_position_detailed.csv读取位置统计数据
func ReadPositionStats(inputDir, id string) ([]PositionStats, error) {
	// 构建position_detailed.csv文件路径
	csvPath := filepath.Join(inputDir, "mutation_stats", id+"_position_detailed.csv")

	// 打开文件
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("打开%s_position_detailed.csv失败: %w", id, err)
	}
	defer file.Close()

	// 读取CSV文件
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 允许不同行有不同数量的字段
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("读取%s_position_detailed.csv失败: %w", id, err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("%s_position_detailed.csv文件内容不足", id)
	}

	// 找到所需列的索引
	header := records[0]
	posIndex := -1
	depthIndex := -1
	matchPureIndex := -1
	mismatchPureIndex := -1
	mismatchWithInsIndex := -1
	insertionIndex := -1
	deletionIndex := -1

	for i, col := range header {
		switch col {
		case "pos":
			posIndex = i
		case "depth":
			depthIndex = i
		case "match_pure":
			matchPureIndex = i
		case "mismatch_pure":
			mismatchPureIndex = i
		case "mismatch_with_ins":
			mismatchWithInsIndex = i
		case "insertion":
			insertionIndex = i
		case "deletion":
			deletionIndex = i
		}
	}

	// 检查是否找到所有需要的列
	if posIndex == -1 || depthIndex == -1 || matchPureIndex == -1 ||
		mismatchPureIndex == -1 || mismatchWithInsIndex == -1 ||
		insertionIndex == -1 || deletionIndex == -1 {
		return nil, fmt.Errorf("total_position_detailed.csv文件缺少必要的列")
	}

	// 解析数据
	var positionStats []PositionStats
	for i := 1; i < len(records); i++ {
		record := records[i]
		if len(record) <= max(posIndex, depthIndex, matchPureIndex,
			mismatchPureIndex, mismatchWithInsIndex,
			insertionIndex, deletionIndex) {
			continue // 跳过列数不足的行
		}

		// 解析数值
		pos, err := strconv.Atoi(record[posIndex])
		if err != nil {
			continue
		}

		depth, err := strconv.ParseFloat(record[depthIndex], 64)
		if err != nil || depth == 0 {
			continue
		}

		matchPure, err := strconv.ParseFloat(record[matchPureIndex], 64)
		if err != nil {
			continue
		}

		mismatchPure, err := strconv.ParseFloat(record[mismatchPureIndex], 64)
		if err != nil {
			continue
		}

		mismatchWithIns, err := strconv.ParseFloat(record[mismatchWithInsIndex], 64)
		if err != nil {
			continue
		}

		insertion, err := strconv.ParseFloat(record[insertionIndex], 64)
		if err != nil {
			continue
		}

		deletion, err := strconv.ParseFloat(record[deletionIndex], 64)
		if err != nil {
			continue
		}

		// 计算统计值
		avgYield := (matchPure / depth) * 100
		avgDeletion := (deletion / depth) * 100
		avgMutation := ((mismatchPure + mismatchWithIns) / depth) * 100
		avgInsertion := (insertion / depth) * 100

		// 添加到结果列表
		positionStats = append(positionStats, PositionStats{
			Pos:          pos,
			AvgYield:     avgYield,
			AvgDeletion:  avgDeletion,
			AvgMutation:  avgMutation,
			AvgInsertion: avgInsertion,
		})
	}

	return positionStats, nil
}

// max 返回多个整数中的最大值
func max(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	maxVal := vals[0]
	for _, v := range vals {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// ReadSplitSummary 从split_summary.txt读取总处理reads数
func ReadSplitSummary(inputDir string) (int, error) {
	// 构建split_summary.txt文件路径
	summaryPath := filepath.Join(inputDir, "split_summary.txt")

	// 打开文件
	file, err := os.Open(summaryPath)
	if err != nil {
		return 0, fmt.Errorf("打开split_summary.txt失败: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// 查找"总处理reads数:"行
		if strings.Contains(line, "总处理reads数:") {
			// 提取数值
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				// 去除空格和逗号
				numStr := strings.TrimSpace(parts[1])
				numStr = strings.ReplaceAll(numStr, ",", "")
				// 转换为整数
				num, err := strconv.Atoi(numStr)
				if err != nil {
					return 0, fmt.Errorf("解析总处理reads数失败: %w", err)
				}
				return num, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("读取split_summary.txt失败: %w", err)
	}

	return 0, fmt.Errorf("未找到总处理reads数")
}

// ReadYieldStats 从read_type_by_sample.csv读取收率和错误统计数据
func ReadYieldStats(inputDir string) (map[string]float64, map[string]float64, map[string]float64, map[string]float64, error) {
	csvPath := filepath.Join(inputDir, "mutation_stats", "read_type_by_sample.csv")
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("打开read_type_by_sample.csv失败: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 允许不同行有不同数量的字段
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("读取read_type_by_sample.csv失败: %w", err)
	}

	yieldStats := make(map[string]float64)
	deletionStats := make(map[string]float64)
	mutationStats := make(map[string]float64)
	insertionStats := make(map[string]float64)
	var matchIndex int = -1
	var goodAlignedReadsIndex int = -1
	var deleteReadsIndex int = -1
	var substitutionReadsIndex int = -1
	var insertReadsIndex int = -1

	// 找到匹配列和GoodAlignedReads列的索引
	if len(records) > 0 {
		for i, header := range records[0] {
			if header == "匹配" {
				matchIndex = i
			}
			if header == "GoodAlignedReads" {
				goodAlignedReadsIndex = i
			}
			if header == "DeleteReads" {
				deleteReadsIndex = i
			}
			if header == "SubstitutionReads" {
				substitutionReadsIndex = i
			}
			if header == "InsertReads" {
				insertReadsIndex = i
			}
		}
	}

	// 检查是否找到必要的列
	if matchIndex == -1 || goodAlignedReadsIndex == -1 || deleteReadsIndex == -1 || substitutionReadsIndex == -1 || insertReadsIndex == -1 {
		return nil, nil, nil, nil, fmt.Errorf("在read_type_by_sample.csv中未找到必要的列")
	}

	// 读取每个样本的收率和错误数据
	for i, record := range records {
		// 跳过空行和表头
		if i == 0 || len(record) == 0 || (len(record) > 0 && strings.TrimSpace(record[0]) == "") {
			continue
		}

		sampleName := strings.TrimSpace(record[0])
		if sampleName == "" {
			continue
		}

		if len(record) > matchIndex && len(record) > goodAlignedReadsIndex && len(record) > deleteReadsIndex && len(record) > substitutionReadsIndex && len(record) > insertReadsIndex {
			// 计算收率：匹配/GoodAlignedReads
			matchStr := strings.TrimSpace(record[matchIndex])
			goodAlignedReadsStr := strings.TrimSpace(record[goodAlignedReadsIndex])
			deleteReadsStr := strings.TrimSpace(record[deleteReadsIndex])
			substitutionReadsStr := strings.TrimSpace(record[substitutionReadsIndex])
			insertReadsStr := strings.TrimSpace(record[insertReadsIndex])

			if matchStr != "" && goodAlignedReadsStr != "" && deleteReadsStr != "" && substitutionReadsStr != "" && insertReadsStr != "" {
				match, err1 := strconv.ParseFloat(matchStr, 64)
				goodAlignedReads, err2 := strconv.ParseFloat(goodAlignedReadsStr, 64)
				deleteReads, err3 := strconv.ParseFloat(deleteReadsStr, 64)
				substitutionReads, err4 := strconv.ParseFloat(substitutionReadsStr, 64)
				insertReads, err5 := strconv.ParseFloat(insertReadsStr, 64)

				if err1 == nil && err2 == nil && goodAlignedReads > 0 {
					yield := (match / goodAlignedReads) * 100 // 转换为百分比
					yieldStats[sampleName] = yield

					if err3 == nil {
						deletion := (deleteReads / goodAlignedReads) * 100 // 转换为百分比
						deletionStats[sampleName] = deletion
					}

					if err4 == nil {
						mutation := (substitutionReads / goodAlignedReads) * 100 // 转换为百分比
						mutationStats[sampleName] = mutation
					}

					if err5 == nil {
						insertion := (insertReads / goodAlignedReads) * 100 // 转换为百分比
						insertionStats[sampleName] = insertion
					}
				}
			}
		}
	}

	return yieldStats, deletionStats, mutationStats, insertionStats, nil
}

// CalculateYieldStatistics 计算收率统计值
func CalculateYieldStatistics(yieldStats map[string]float64) (float64, float64, float64, float64, int, int) {
	var yields []float64
	for _, yield := range yieldStats {
		yields = append(yields, yield)
	}

	if len(yields) == 0 {
		return 0, 0, 0, 0, 0, 0
	}

	avgYield, stdYield := meanStdDev(yields)
	medYield := median(yields)
	q1Yield := quartile(yields)

	lt1Count, lt5Count := 0, 0
	for _, yield := range yields {
		if yield < 1 {
			lt1Count++
		}
		if yield < 5 {
			lt5Count++
		}
	}

	return avgYield, stdYield, medYield, q1Yield, lt1Count, lt5Count
}

// ReadBOMFile 从BOM.xlsx文件读取孔位信息
func ReadBOMFile(bomFile string) (map[string]*Well, error) {
	// 由于Go标准库不直接支持读取Excel文件，我们假设BOM.xlsx已经被转换为txt文件
	// 实际项目中可能需要使用第三方库如github.com/360EntSecGroup-Skylar/excelize
	txtFile := bomFile + ".引物订购单.txt"
	file, err := os.Open(txtFile)
	if err != nil {
		return nil, fmt.Errorf("打开BOM文件失败: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	wellMap := make(map[string]*Well) // key: 位置, value: 孔位信息

	// 读取表头，确定各列的索引
	var positionIndex, primerNameIndex, isOverlapIndex, predictedYieldIndex, sequenceIndex int
	positionIndex = -1
	primerNameIndex = -1
	isOverlapIndex = -1
	predictedYieldIndex = -1
	sequenceIndex = -1

	if scanner.Scan() {
		headerLine := scanner.Text()
		headers := strings.Split(headerLine, "\t")
		for i, header := range headers {
			switch strings.TrimSpace(header) {
			case "位置":
				positionIndex = i
			case "引物名称":
				primerNameIndex = i
			case "是否重合":
				isOverlapIndex = i
			case "预测收率":
				predictedYieldIndex = i
			case "序列":
				sequenceIndex = i
			}
		}
	}

	// 检查是否找到必要的列
	if positionIndex == -1 || primerNameIndex == -1 {
		return nil, fmt.Errorf("在BOM文件中未找到'位置'或'引物名称'列")
	}

	// 读取数据行
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, "\t")

		// 确保字段数量足够
		if len(fields) <= positionIndex || len(fields) <= primerNameIndex {
			continue
		}

		position := strings.TrimSpace(fields[positionIndex])
		primerName := strings.TrimSpace(fields[primerNameIndex])

		// 跳过引物名称为"0"的行
		if primerName == "0" {
			continue
		}

		if position != "" && primerName != "" {
			well := &Well{
				Position:  position,
				Name:      primerName,
				IsOverlap: false, // 默认值
			}

			// 读取是否重合列
			if isOverlapIndex != -1 && len(fields) > isOverlapIndex {
				isOverlapStr := strings.TrimSpace(fields[isOverlapIndex])
				if isOverlapStr == "是" || isOverlapStr == "1" || isOverlapStr == "true" {
					well.IsOverlap = true
				}
			}

			// 读取预测收率列
			if predictedYieldIndex != -1 && len(fields) > predictedYieldIndex {
				predictedYieldStr := strings.TrimSpace(fields[predictedYieldIndex])
				if predictedYieldStr != "" {
					predictedYield, err := strconv.ParseFloat(predictedYieldStr, 64)
					if err == nil {
						well.PredictedYield = predictedYield
					}
				}
			}

			// 读取序列列
			if sequenceIndex != -1 && len(fields) > sequenceIndex {
				well.Sequence = strings.TrimSpace(fields[sequenceIndex])
			}

			wellMap[position] = well
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取BOM文件失败: %w", err)
	}

	return wellMap, nil
}

// updateWellInfo 根据BOM文件更新孔位信息
func updateWellInfo(report *ReportData, wellMap map[string]*Well) {
	// 清空现有的孔位信息
	report.Wells = []*Well{}

	// 计算序列长度的最大值
	maxSequenceLength := 0

	// 根据BOM文件创建新的孔位信息
	for _, well := range wellMap {
		// 解析位置信息，如 "A1" -> 行1，列字母A
		row, colLetter, err := parseWellPosition(well.Position)
		if err != nil {
			continue
		}

		// 计算序列长度
		sequenceLength := len(well.Sequence)
		if sequenceLength > maxSequenceLength {
			maxSequenceLength = sequenceLength
		}

		// 设置行和列信息
		well.Row = row
		well.ColLetter = colLetter

		// 初始化收率和错误数据
		well.Yield = 0     // 默认收率为0
		well.Deletion = 0  // 默认缺失为0
		well.Mutation = 0  // 默认突变为0
		well.Insertion = 0 // 默认插入为0

		report.Wells = append(report.Wells, well)
	}

	// 更新well_count为实际well个数
	report.WellCount = len(report.Wells)

	// 更新synthesis_length为序列长度的最大值
	report.SynthesisLength = maxSequenceLength
}

// parseWellPosition 解析孔位位置，如 "A1" -> 行1，列字母A
func parseWellPosition(position string) (int, string, error) {
	// 提取列字母和行号
	var colLetter string
	var rowStr string

	for i, r := range position {
		if (r >= 'A' && r <= 'H') || (r >= 'a' && r <= 'h') {
			colLetter = string(r)
			rowStr = position[i+1:]
			break
		}
	}

	if colLetter == "" || rowStr == "" {
		return 0, "", fmt.Errorf("无效的孔位位置: %s", position)
	}

	row, err := strconv.Atoi(rowStr)
	if err != nil {
		return 0, "", fmt.Errorf("无效的行号: %s", rowStr)
	}

	// 转换为大写字母
	colLetter = strings.ToUpper(colLetter)

	return row, colLetter, nil
}
