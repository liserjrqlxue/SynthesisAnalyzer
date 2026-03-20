package report

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
	"gonum.org/v1/gonum/stat"
)

// Processor 数据处理器
type Processor struct {
	config *Config
}

// NewProcessor 创建新的数据处理器
func NewProcessor(config *Config) *Processor {
	return &Processor{
		config: config,
	}
}

// LoadData 加载并处理数据
func (p *Processor) LoadData() (*ReportData, error) {
	// 流式读取和解析JSON
	report, err := p.loadJSON()
	if err != nil {
		return nil, err
	}

	// 验证报告数据
	if err := p.validateReportData(report); err != nil {
		log.Printf("警告：报告数据验证失败: %v", err)
	}

	// 如果提供了BOM.xlsx文件，读取并更新孔位信息
	if p.config.BomFile != "" {
		// 解析 Batch 信息
		batchID, synthesisDate, instrumentID, err := ParseBatchInfo(p.config.BomFile)
		if err != nil {
			log.Printf("警告：解析 BOM 文件路径失败: %v", err)
		} else {
			log.Printf("从 BOM 文件解析得到: BatchID=%s, 合成日期=%s, 仪器号=%s", batchID, synthesisDate, instrumentID)
			// 使用这些信息更新报告数据
			report.SynthesisDate = synthesisDate
			report.InstrumentID = instrumentID
			report.BatchID = batchID
		}
		if err := p.updateWellInfo(report); err != nil {
			log.Printf("警告：更新孔位信息失败: %v", err)
		} else {
			log.Printf("成功更新孔位信息")
		}
	}

	// 如果提供了mutation_stats目录，读取并更新数据
	if p.config.MutationStatsDir != "" {
		if err := p.updateMutationStats(report); err != nil {
			log.Printf("警告：更新突变统计数据失败: %v", err)
		} else {
			log.Printf("成功更新突变统计数据")
		}
	}

	return report, nil
}

// loadJSON 加载JSON文件或创建默认报告数据
func (p *Processor) loadJSON() (*ReportData, error) {
	// 如果没有指定输入文件，创建默认的ReportData结构
	if p.config.InputFile == "" {
		return &ReportData{
			ReportTitle:         "TIESyno-96合成仪下机报告",
			SynthesisProcessVer: "V3.0",
			SEC1ProcessVer:      "SECV2.0",
			BarcodeReads:        0,
			FilteredReads:       0,
			MatchedReads:        0,
			Wells:               []*Well{},
			ErrorStatsRef:       []ErrorStatRef{},
			Summary: Summary{
				BasicInfo: BasicInfo{
					SynthesisProcessVer: "V3.0",
					SEC1ProcessVer:      "SECV1.0",
				},
				Statistics: SummaryStats{},
				ErrorStats: []ErrorStat{},
			},
			CycleAnalysis: CycleAnalysis{},
			Appendix:      Appendix{},
		}, nil
	}

	// 否则，加载指定的JSON文件
	file, err := os.Open(p.config.InputFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var report ReportData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&report); err != nil {
		return nil, err
	}

	// 如果报告标题为空，设置默认值
	if report.ReportTitle == "" {
		report.ReportTitle = "TIESyno-96合成仪下机报告"
	}
	// 如果合成工艺版本为空，设置默认值
	if report.SynthesisProcessVer == "" {
		report.SynthesisProcessVer = "V3.0"
	}
	// 如果SEC1工艺版本为空，设置默认值
	if report.SEC1ProcessVer == "" {
		report.SEC1ProcessVer = "SECV1.0"
	}
	// 确保Summary中的版本信息也同步
	if report.Summary.BasicInfo.SynthesisProcessVer == "" {
		report.Summary.BasicInfo.SynthesisProcessVer = "V3.0"
	}
	if report.Summary.BasicInfo.SEC1ProcessVer == "" {
		report.Summary.BasicInfo.SEC1ProcessVer = "SECV1.0"
	}

	return &report, nil
}

// validateReportData 验证报告数据
func (p *Processor) validateReportData(report *ReportData) error {
	if report.ReportTitle == "" {
		return fmt.Errorf("报告标题为空")
	}
	return nil
}

// updateWellInfo 根据BOM文件更新孔位信息
func (p *Processor) updateWellInfo(report *ReportData) error {
	// 清空现有的孔位信息
	report.Wells = []*Well{}

	wellMap, err := ReadBOMFile(p.config.BomFile)
	if err != nil {
		return err
	}

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

	return nil
}

// updateMutationStats 更新突变统计数据
func (p *Processor) updateMutationStats(report *ReportData) error {
	// 验证目录是否存在
	if _, err := os.Stat(p.config.MutationStatsDir); os.IsNotExist(err) {
		return fmt.Errorf("mutation_stats目录不存在: %s", p.config.MutationStatsDir)
	}

	// 读取子类型统计数据
	subtypeStats, err := ReadSubtypeStats(p.config.MutationStatsDir)
	if err != nil {
		log.Printf("警告：读取子类型统计数据失败: %v", err)
	} else {
		// 更新错误统计
		p.updateErrorStats(report, subtypeStats)
		log.Printf("成功更新错误统计数据")
	}

	// 读取收率和错误统计数据
	yieldStats, deletionStats, mutationStats, insertionStats, err := ReadYieldStats(p.config.MutationStatsDir)
	if err != nil {
		log.Printf("警告：读取收率和错误统计数据失败: %v", err)
	} else {
		// 更新收率和错误数据
		p.updateYieldData(report, yieldStats, deletionStats, mutationStats, insertionStats)
		log.Printf("成功更新收率和错误数据")
	}

	// 读取split_summary.txt中的总处理reads数、测序时间、过滤拼接后reads数和成功匹配reads数
	totalReads, sequencingDate, filteredReads, matchedReads, err := ReadSplitSummary(p.config.MutationStatsDir)
	if err != nil {
		log.Printf("警告：读取split_summary.txt失败: %v", err)
	} else {
		// 更新BarcodeReads字段（现在存储总处理reads数）
		report.BarcodeReads = totalReads
		log.Printf("从split_summary.txt更新总处理reads数: %d", totalReads)

		// 更新测序时间字段
		if sequencingDate != "" {
			report.SequencingDate = sequencingDate
			log.Printf("从split_summary.txt更新SequencingDate: %s", sequencingDate)
		}

		// 更新过滤拼接后reads数字段
		report.FilteredReads = filteredReads
		log.Printf("从split_summary.txt更新过滤拼接后reads数: %d", filteredReads)

		// 更新成功匹配reads数字段
		report.MatchedReads = matchedReads
		log.Printf("从split_summary.txt更新成功匹配reads数: %d", matchedReads)
	}

	// 读取位置统计数据
	positionStats, err := ReadPositionStats(p.config.MutationStatsDir, "total")
	if err != nil {
		log.Printf("警告：读取位置统计数据失败: %v", err)
	} else {
		log.Printf("成功读取位置统计数据，共%d个位置", len(positionStats))
		// 将positionStats存储在report结构中，以便在生成报告时使用
		report.PositionStats = positionStats
	}

	// 更新每个孔的位置统计数据
	p.updatePositionStats(report, p.config.MutationStatsDir)
	log.Printf("成功更新每个孔的位置统计数据")

	return nil
}

// updateErrorStats 更新错误统计数据
func (p *Processor) updateErrorStats(report *ReportData, subtypeStats map[string]float64) {
	// 清空现有的错误统计
	report.Summary.ErrorStats = []ErrorStat{}

	// 按照固定顺序添加子类型错误统计
	for _, subtype := range errorTypes {
		if value, ok := subtypeStats[subtype]; ok {
			report.Summary.ErrorStats = append(report.Summary.ErrorStats, ErrorStat{
				ErrorType: subtype,
				Data:      &value,
			})
		} else {
			// 如果没有值，添加一个空的错误统计条目
			report.Summary.ErrorStats = append(report.Summary.ErrorStats, ErrorStat{
				ErrorType: subtype,
				Data:      nil,
			})
		}
	}
}

// updateYieldData 更新收率数据
func (p *Processor) updateYieldData(report *ReportData, yieldStats map[string]float64, deletionStats map[string]float64, mutationStats map[string]float64, insertionStats map[string]float64) {
	// 收集有统计数据的收率值
	var yields []float64
	for _, well := range report.Wells {
		hasStats := false
		if yield, ok := yieldStats[well.Name]; ok {
			well.Yield = yield
			yields = append(yields, yield)
			hasStats = true
		}
		if deletion, ok := deletionStats[well.Name]; ok {
			well.Deletion = deletion
			hasStats = true
		}
		if mutation, ok := mutationStats[well.Name]; ok {
			well.Mutation = mutation
			hasStats = true
		}
		if insertion, ok := insertionStats[well.Name]; ok {
			well.Insertion = insertion
			hasStats = true
		}
		well.HasStats = hasStats
	}

	// 计算收率统计值
	avgYield, stdYield, medYield, q1Yield, lt1Count, lt5Count := CalculateYieldStatistics(yields)

	// 更新概要统计
	report.Summary.Statistics.AvgYield = avgYield
	report.Summary.Statistics.YieldStddev = stdYield
	report.Summary.Statistics.YieldMedian = medYield
	report.Summary.Statistics.YieldQuartile = q1Yield
	report.Summary.Statistics.YieldLt1Count = lt1Count
	report.Summary.Statistics.YieldLt5Count = lt5Count
}

// updatePositionStats 更新每个孔的位置统计数据并计算批次均值
func (p *Processor) updatePositionStats(report *ReportData, mutationStatsDir string) {
	// 并发读取每个孔位的位置统计数据
	var wg sync.WaitGroup
	wg.Add(len(report.Wells))

	for _, well := range report.Wells {
		go func(w *Well) {
			defer wg.Done()
			// 读取该孔位的位置统计数据
			positionStats, err := ReadPositionStats(mutationStatsDir, w.Name)
			if err != nil {
				log.Printf("警告：读取孔位%s的位置统计数据失败: %v", w.Name, err)
			} else {
				slog.Debug("成功读取位置统计数据", "孔位", w.Name, "孔数", len(positionStats))
				// 将位置统计数据存储在孔位结构中
				w.PositionStats = positionStats
			}
		}(well)
	}

	wg.Wait()

	// 计算批次均值位置统计数据
	report.PositionStatsMean = p.calculateBatchMeanPositionStats(report.Wells)
}

// calculateBatchMeanPositionStats 计算批次均值位置统计数据
func (p *Processor) calculateBatchMeanPositionStats(wells []*Well) []PositionStats {
	if len(wells) == 0 {
		return nil
	}

	// 收集所有位置
	positionMap := make(map[int]struct{})
	for _, well := range wells {
		for _, ps := range well.PositionStats {
			positionMap[ps.Pos] = struct{}{}
		}
	}

	// 将位置转换为切片并排序
	var positions []int
	for pos := range positionMap {
		positions = append(positions, pos)
	}
	sort.Ints(positions)

	// 计算每个位置的均值
	var result []PositionStats
	for _, pos := range positions {
		var totalYield, totalDeletion, totalMutation, totalInsertion float64
		var count int

		for _, well := range wells {
			for _, ps := range well.PositionStats {
				if ps.Pos == pos {
					totalYield += ps.AvgYield
					totalDeletion += ps.AvgDeletion
					totalMutation += ps.AvgMutation
					totalInsertion += ps.AvgInsertion
					count++
					break
				}
			}
		}

		if count > 0 {
			result = append(result, PositionStats{
				Pos:          pos,
				AvgYield:     totalYield / float64(count),
				AvgDeletion:  totalDeletion / float64(count),
				AvgMutation:  totalMutation / float64(count),
				AvgInsertion: totalInsertion / float64(count),
			})
		}
	}

	return result
}

// ReadBOMFile 从BOM.xlsx文件读取孔位信息
func ReadBOMFile(bomFile string) (map[string]*Well, error) {
	var (
		sheetName = "引物订购单"
	)

	// 打开Excel文件
	f, err := excelize.OpenFile(bomFile)
	if err != nil {
		return nil, fmt.Errorf("打开BOM文件失败: %w", err)
	}
	defer f.Close()

	// 获取所有行
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("读取BOM文件失败: %w", err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("BOM文件内容不足")
	}

	wellMap := make(map[string]*Well) // key: 位置, value: 孔位信息

	// 读取表头，确定各列的索引
	var positionIndex, primerNameIndex, isOverlapIndex, predictedYieldIndex, sequenceIndex int
	positionIndex = -1
	primerNameIndex = -1
	isOverlapIndex = -1
	predictedYieldIndex = -1
	sequenceIndex = -1

	headers := rows[0]
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

	// 检查是否找到必要的列
	if positionIndex == -1 || primerNameIndex == -1 {
		return nil, fmt.Errorf("在BOM文件中未找到'位置'或'引物名称'列")
	}

	// 读取数据行
	for i := 1; i < len(rows); i++ {
		row := rows[i]

		// 确保字段数量足够
		if len(row) <= positionIndex || len(row) <= primerNameIndex {
			continue
		}

		position := strings.TrimSpace(row[positionIndex])
		primerName := strings.TrimSpace(row[primerNameIndex])

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
			if isOverlapIndex != -1 && len(row) > isOverlapIndex {
				isOverlapStr := strings.TrimSpace(row[isOverlapIndex])
				if isOverlapStr == "是" || isOverlapStr == "1" || isOverlapStr == "true" {
					well.IsOverlap = true
				}
			}

			// 读取预测收率列
			if predictedYieldIndex != -1 && len(row) > predictedYieldIndex {
				predictedYieldStr := strings.TrimSpace(row[predictedYieldIndex])
				if predictedYieldStr != "" {
					predictedYield, err := strconv.ParseFloat(predictedYieldStr, 64)
					if err == nil {
						well.PredictedYield = predictedYield
					}
				}
			}

			// 读取序列列
			if sequenceIndex != -1 && len(row) > sequenceIndex {
				well.Sequence = strings.TrimSpace(row[sequenceIndex])
			}

			wellMap[position] = well
		}
	}

	return wellMap, nil
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

// ReadSplitSummary 从split_summary.txt读取总处理reads数、测序时间、过滤拼接后reads数和成功匹配reads数
func ReadSplitSummary(inputDir string) (int, string, int, int, error) {
	// 构建split_summary.txt文件路径
	summaryPath := filepath.Join(inputDir, "split_summary.txt")

	// 打开文件
	file, err := os.Open(summaryPath)
	if err != nil {
		return 0, "", 0, 0, fmt.Errorf("打开split_summary.txt失败: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	scanner := bufio.NewScanner(file)
	var totalReads int
	var sequencingDate string
	var filteredReads int
	var matchedReads int

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
					return 0, "", 0, 0, fmt.Errorf("解析总处理reads数失败: %w", err)
				}
				totalReads = num
			}
		}
		// 查找"测序时间:"行
		if strings.Contains(line, "测序时间:") {
			// 提取时间
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				sequencingDate = strings.TrimSpace(parts[1])
			}
		}
		// 查找"过滤拼接后reads数:"行
		if strings.Contains(line, "过滤拼接后reads数:") {
			// 提取数值
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				// 去除空格和逗号
				numStr := strings.TrimSpace(parts[1])
				numStr = strings.ReplaceAll(numStr, ",", "")
				// 转换为整数
				num, err := strconv.Atoi(numStr)
				if err != nil {
					return 0, "", 0, 0, fmt.Errorf("解析过滤拼接后reads数失败: %w", err)
				}
				filteredReads = num
			}
		}
		// 查找"成功匹配reads数:"行
		if strings.Contains(line, "成功匹配reads数:") {
			// 提取数值
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				// 去除空格、逗号和括号内的百分比
				numStr := strings.TrimSpace(parts[1])
				numStr = strings.ReplaceAll(numStr, ",", "")
				// 提取数字部分（去除括号内的百分比）
				if idx := strings.Index(numStr, " ("); idx != -1 {
					numStr = numStr[:idx]
				}
				// 转换为整数
				num, err := strconv.Atoi(numStr)
				if err != nil {
					return 0, "", 0, 0, fmt.Errorf("解析成功匹配reads数失败: %w", err)
				}
				matchedReads = num
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, "", 0, 0, fmt.Errorf("读取split_summary.txt失败: %w", err)
	}

	if totalReads == 0 {
		return 0, "", 0, 0, fmt.Errorf("未找到总处理reads数")
	}

	return totalReads, sequencingDate, filteredReads, matchedReads, nil
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

func CalculateYieldStatistics(yields []float64) (float64, float64, float64, float64, int, int) {
	if len(yields) == 0 {
		return 0, 0, 0, 0, 0, 0
	}
	sort.Float64s(yields)

	meanYield, stdYield := stat.MeanStdDev(yields, nil)
	medYield := stat.Quantile(0.5, stat.LinInterp, yields, nil)
	q1Yield := stat.Quantile(0.25, stat.LinInterp, yields, nil)

	lt1Count, lt5Count := 0, 0
	for _, yield := range yields {
		if yield < 1 {
			lt1Count++
		}
		if yield < 5 {
			lt5Count++
		}
	}

	return meanYield, stdYield, medYield, q1Yield, lt1Count, lt5Count
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
