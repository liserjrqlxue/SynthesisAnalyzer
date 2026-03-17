package report

import (
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"gonum.org/v1/gonum/stat"
)

// Generator 报告生成器
type Generator struct {
	config *Config
}

// NewGenerator 创建新的报告生成器
func NewGenerator(config *Config) *Generator {
	return &Generator{
		config: config,
	}
}

// GenerateHTML 生成HTML报告
func (g *Generator) GenerateHTML(data *ReportData) string {
	// 预分配足够的容量，减少内存分配
	b := &strings.Builder{}
	b.Grow(10 * 1024 * 1024) // 预分配10MB容量

	// 获取输出目录
	outputDir := "."
	if g.config.OutputFile != "" {
		outputDir = filepath.Dir(g.config.OutputFile)
	}

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("警告：创建输出目录失败: %v", err)
	}

	// HTML 头部
	g.writeHTMLHeader(b, data.ReportTitle)

	// 构建二维孔索引
	plate := g.buildWellPlate(data.Wells)
	batchID := g.makeBatchID(data.SynthesisDate, data.InstrumentID)

	// 1. Summary
	g.writeSummarySection(b, data, plate, batchID)

	// 2. 合成板位分析
	g.writePlateAnalysisSection(b, data, plate, batchID)

	// 3. 合成轮次分析
	g.writeCycleAnalysisSection(b, data, outputDir)

	// 4. 附录
	g.writeAppendixSection(b, data, batchID)

	// HTML 尾部
	g.writeHTMLFooter(b)

	return b.String()
}

// writeHTMLHeader 写入HTML头部
func (g *Generator) writeHTMLHeader(b *strings.Builder, reportTitle string) {
	b.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>`)
	b.WriteString(html.EscapeString(reportTitle))
	b.WriteString(`</title>
<script src="https://go-echarts.github.io/go-echarts-assets/assets/echarts.min.js"></script>
<style>
body { 
	font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
	margin: 2em;
	line-height: 1.6;
	color: #333;
}
h1 { 
	color: #2c3e50;
	font-size: 2.5em;
	margin-bottom: 1em;
	text-align: center;
}
h2 { 
	border-bottom: 2px solid #3498db;
	padding-bottom: 0.5em;
	margin-top: 2em;
	margin-bottom: 1.5em;
	color: #2c3e50;
    page-break-before: always;
}
h2:first-of-type {
    page-break-before: auto;   /* 第一个标题前不分页 */
}
h3 { 
	color: #34495e;
	margin-top: 1.5em;
	margin-bottom: 1em;
}
table { 
	margin: 1em 0;
	border-collapse: collapse;
	width: 100%;
	max-width: 1000px;
	page-break-inside: avoid; /* 防止表格整体跨页 */
	box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}
table th, table td {
	padding: 8px 12px;
	text-align: center;
	border: 1px solid #ddd;
}
table th {
	background-color: #f8f9fa;
	font-weight: 600;
	color: #2c3e50;
}
table tr:nth-child(even) {
	background-color: #f8f9fa;
}
table tr:hover {
	background-color: #e3f2fd;
}
thead {
    display: table-header-group;
}
tr { page-break-inside: avoid; } /* 防止单个行跨页 */
.chart-container {
	width: 100%;
	max-width: 1000px;
	margin: 0 auto;
	margin-top: 30px;
	margin-bottom: 30px;
	padding: 20px;
	background-color: #f8f9fa;
	border-radius: 8px;
	box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}
.chart-item {
	width: 100%;
	height: 500px;
}
/* 保证整个区块（标题+表格/图表）尽量在同一页 */
.section {
    page-break-inside: avoid;   /* 防止内部被拆分 */
    margin-bottom: 2em;
    padding: 20px;
    background-color: #ffffff;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.05);
}
/* 响应式设计 */
@media (max-width: 768px) {
	body {
		margin: 1em;
	}
	h1 {
		font-size: 2em;
	}
	table {
		font-size: 0.9em;
	}
	.chart-container {
		padding: 10px;
	}
	.chart-item {
		height: 400px;
	}
}
</style>
</head>
<body>
`)
	b.WriteString(`<h1>`)
	b.WriteString(html.EscapeString(reportTitle))
	b.WriteString(`</h1>
`)
}

// writeHTMLFooter 写入HTML尾部
func (g *Generator) writeHTMLFooter(b *strings.Builder) {
	b.WriteString(`</body>
</html>`)
}

// writeSummarySection 写入摘要部分
func (g *Generator) writeSummarySection(b *strings.Builder, data *ReportData, plate [Rows][Cols]*Well, batchID string) {
	b.WriteString("<h2>1. Summary</h2>\n")

	// 基本信息
	b.WriteString(`<div class="section">` + "\n")
	b.WriteString("<h3>基本信息</h3>\n")
	b.WriteString(`<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;; min-width: 800px;">` + "\n")
	b.WriteString(`<colgroup><col style="width: 240px;"></colgroup>`)
	b.WriteString(`<colgroup><col style="width: 240px;"></colgroup>`)
	b.WriteString(fmt.Sprintf("<tr><td>合成日期</td><td>%s</td></tr>\n", data.SynthesisDate))
	b.WriteString(fmt.Sprintf("<tr><td>仪器号</td><td>%s</td></tr>\n", data.InstrumentID))
	b.WriteString(fmt.Sprintf("<tr><td>合成孔数</td><td>%d</td></tr>\n", data.WellCount))
	b.WriteString(fmt.Sprintf("<tr><td>合成长度</td><td>%dnt</td></tr>\n", data.SynthesisLength))
	b.WriteString(fmt.Sprintf("<tr><td>合成工艺版本</td><td>%s</td></tr>\n", data.SynthesisProcessVer))
	b.WriteString(fmt.Sprintf("<tr><td>SEC1工艺版本</td><td>%s</td></tr>\n", data.SEC1ProcessVer))
	b.WriteString(fmt.Sprintf("<tr><td>测序日期</td><td>%s</td></tr>\n", data.SequencingDate))
	b.WriteString(fmt.Sprintf("<tr><td>Barcode数据量（reads）</td><td>%d</td></tr>\n", data.BarcodeReads))

	// 统计概要
	stats := data.Summary.Statistics

	// 建立参考值映射
	refMap := make(map[string]*float64)
	for _, ref := range data.ErrorStatsRef {
		refMap[ref.ErrorType] = ref.Reference
	}

	// 平均收率 - 根据参考值是否存在来决定输出格式
	if refVal, ok := refMap["平均收率"]; ok && refVal != nil && *refVal > 0 {
		b.WriteString(fmt.Sprintf("<tr><td>平均收率</td><td>%.2f%% (参考值%.2f%%)</td></tr>\n", stats.AvgYield, *refVal))
	} else {
		b.WriteString(fmt.Sprintf("<tr><td>平均收率</td><td>%.2f%%</td></tr>\n", stats.AvgYield))
	}
	// 收率标准差 - 根据参考值是否存在来决定输出格式
	if refVal, ok := refMap["收率标准差"]; ok && refVal != nil && *refVal > 0 {
		b.WriteString(fmt.Sprintf("<tr><td>收率标准差</td><td>%.2f%% (参考值%.2f%%)</td></tr>\n", stats.YieldStddev, *refVal))
	} else {
		b.WriteString(fmt.Sprintf("<tr><td>收率标准差</td><td>%.2f%%</td></tr>\n", stats.YieldStddev))
	}

	b.WriteString(fmt.Sprintf("<tr><td>收率标准差</td><td>%.2f%%</td></tr>\n", stats.YieldStddev))
	b.WriteString(fmt.Sprintf("<tr><td>收率中位数</td><td>%.2f%%</td></tr>\n", stats.YieldMedian))
	b.WriteString(fmt.Sprintf("<tr><td>收率四分位数</td><td>%.2f%%</td></tr>\n", stats.YieldQuartile))
	b.WriteString(fmt.Sprintf("<tr><td>收率&lt;1%%片段个数</td><td>%d</td></tr>\n", stats.YieldLt1Count))
	b.WriteString(fmt.Sprintf("<tr><td>收率&lt;5%%片段个数</td><td>%d</td></tr>\n", stats.YieldLt5Count))
	b.WriteString(fmt.Sprintf("<tr><td>预测难度序列</td><td>%d</td></tr>\n", stats.PredictedDifficultSeq))
	b.WriteString(fmt.Sprintf("<tr><td>重合序列</td><td>%d</td></tr>\n", stats.OverlapSequences))
	b.WriteString("</table>\n")
	b.WriteString("<p><small>*注：预测难度序列需确定阈值</small></p>\n")
	b.WriteString("</div>\n") // 结束section

	// 合成错误统计
	b.WriteString(`<div class="section">` + "\n")
	b.WriteString("<h3>合成错误统计</h3>\n")
	b.WriteString(`<table border='1' cellpadding='4' style="table-layout: fixed; width: auto; min-width: 800px;">` + "\n")
	b.WriteString(`<colgroup>` + "\n")
	b.WriteString(`<col style="width: 140px;">` + "\n")
	b.WriteString(`<col style="width: 230px;">` + "\n")
	b.WriteString(`<col style="width: 120px;">` + "\n")
	b.WriteString(`<col style="width: 120px;">` + "\n")
	b.WriteString(`<col style="width: 120px;">` + "\n")
	b.WriteString(`</colgroup>` + "\n")
	b.WriteString("<tr><th>错误类型-EN</th><th>错误类型-CN</th><th>示例</th><th>参考值</th><th>数据</th></tr>\n")

	for _, es := range data.Summary.ErrorStats {
		// 获取错误类型的详细信息
		errorInfo := errorTypeInfoMap[es.ErrorType]

		refVal := refMap[es.ErrorType]
		refStr := ""
		if refVal != nil {
			refStr = fmt.Sprintf("%.2f", *refVal)
		}
		dataStr := ""
		if es.Data != nil {
			dataStr = fmt.Sprintf("%.2f", *es.Data)
		}
		b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			errorInfo.EN, errorInfo.CN, errorInfo.Example, refStr, dataStr))
	}
	b.WriteString("</table>\n")
	b.WriteString("</div>\n") // 结束section
}

// writePlateAnalysisSection 写入板位分析部分
func (g *Generator) writePlateAnalysisSection(b *strings.Builder, data *ReportData, plate [Rows][Cols]*Well, batchID string) {
	b.WriteString("<h2>2. 合成板位分析</h2>\n")

	// 构建重合矩阵（仅在合成排板时需要）
	var overlapMatrix [Rows][Cols]bool
	for i := range Rows {
		for j := range Cols {
			w := plate[i][j]
			if w != nil && w.IsOverlap {
				overlapMatrix[i][j] = true
			}
		}
	}

	// 2.1 合成排板
	namePlate := g.extractFieldPlate(plate, "name")
	g.printPlateTableHTML(b, "2.1 合成排板", "", "标黄引物为重合序列", batchID, namePlate, &overlapMatrix)

	// 2.2 预测合成收率
	predVals := g.extractValues(plate, "predicted_yield")
	meanPred, stdPred := stat.MeanStdDev(predVals, nil)
	predTitle := fmt.Sprintf("2.2 预测合成收率 (%.2f%%±%.2f%%)", meanPred, stdPred)
	predPlate := g.extractFieldPlate(plate, "predicted_yield")
	g.printPlateTableHTML(b, predTitle, "", "", batchID, predPlate, nil)

	// 2.3 收率板位统计
	yieldVals := g.extractValues(plate, "yield")
	meanYield, stdYield2 := stat.MeanStdDev(yieldVals, nil)
	yieldTitle := fmt.Sprintf("2.3 收率板位统计 (%.2f%%±%.2f%%)", meanYield, stdYield2)
	yieldPlate := g.extractFieldPlate(plate, "yield")
	g.printPlateTableHTML(b, yieldTitle, "", "", batchID, yieldPlate, nil)

	// 2.4 缺失板位统计
	delVals := g.extractValues(plate, "deletion")
	meanDel, stdDel := stat.MeanStdDev(delVals, nil)
	delTitle := fmt.Sprintf("2.4 缺失板位统计 (%.2f%%±%.2f%%)", meanDel, stdDel)
	delPlate := g.extractFieldPlate(plate, "deletion")
	g.printPlateTableHTML(b, delTitle, "", "", batchID, delPlate, nil)

	// 2.5 突变板位统计
	mutVals := g.extractValues(plate, "mutation")
	meanMut, stdMut := stat.MeanStdDev(mutVals, nil)
	mutTitle := fmt.Sprintf("2.5 突变板位统计 (%.2f%%±%.2f%%)", meanMut, stdMut)
	mutPlate := g.extractFieldPlate(plate, "mutation")
	g.printPlateTableHTML(b, mutTitle, "", "", batchID, mutPlate, nil)

	// 2.6 插入板位统计
	insVals := g.extractValues(plate, "insertion")
	meanIns, stdIns := stat.MeanStdDev(insVals, nil)
	insTitle := fmt.Sprintf("2.6 插入板位统计 (%.2f%%±%.2f%%)", meanIns, stdIns)
	insPlate := g.extractFieldPlate(plate, "insertion")
	g.printPlateTableHTML(b, insTitle, "", "", batchID, insPlate, nil)
}

// writeCycleAnalysisSection 写入轮次分析部分
func (g *Generator) writeCycleAnalysisSection(b *strings.Builder, data *ReportData, outputDir string) {
	b.WriteString("<h2>3. 合成轮次分析</h2>\n")

	// 检查是否有位置统计数据
	if len(data.PositionStats) == 0 {
		b.WriteString(`<p>未找到位置统计数据，无法生成合成轮次分析图表。</p>` + "\n")
		return
	}
	// 准备图表数据
	var posData []int
	var yieldData []float64
	var deletionData []float64
	var mutationData []float64
	var insertionData []float64

	for _, stats := range data.PositionStats {
		posData = append(posData, stats.Pos)
		yieldData = append(yieldData, stats.AvgYield)
		deletionData = append(deletionData, stats.AvgDeletion)
		mutationData = append(mutationData, stats.AvgMutation)
		insertionData = append(insertionData, stats.AvgInsertion)
	}

	// 准备批次均值数据
	yieldDataMean := make([]float64, len(posData))
	deletionDataMean := make([]float64, len(posData))
	mutationDataMean := make([]float64, len(posData))
	insertionDataMean := make([]float64, len(posData))

	// 构建位置到索引的映射
	posToIndex := make(map[int]int)
	for i, pos := range posData {
		posToIndex[pos] = i
	}

	// 填充批次均值数据
	for _, stats := range data.PositionStatsMean {
		if idx, ok := posToIndex[stats.Pos]; ok {
			yieldDataMean[idx] = stats.AvgYield
			deletionDataMean[idx] = stats.AvgDeletion
			mutationDataMean[idx] = stats.AvgMutation
			insertionDataMean[idx] = stats.AvgInsertion
		}
	}

	// 生成合成轮次分析图表
	// 收率图表
	yieldDataSets := map[string]struct {
		Data  []float64
		Color string
	}{
		"批次总和 (batch_sum)":  {Data: yieldData, Color: "#FF6384"},
		"批次均值 (batch_mean)": {Data: yieldDataMean, Color: "#4BC0C0"},
	}
	g.generateCycleAnalysisChart(b, "3.1 平均合成收率随轮次变化", "平均合成收率随轮次变化", "yield", posData, yieldDataSets, g.config.UseGoEcharts, g.config.EmbedImage, outputDir)

	// 缺失图表
	deletionDataSets := map[string]struct {
		Data  []float64
		Color string
	}{
		"批次总和 (batch_sum)":  {Data: deletionData, Color: "#FF6384"},
		"批次均值 (batch_mean)": {Data: deletionDataMean, Color: "#4BC0C0"},
	}
	g.generateCycleAnalysisChart(b, "3.2 平均缺失随轮次变化", "平均缺失随轮次变化", "deletion", posData, deletionDataSets, g.config.UseGoEcharts, g.config.EmbedImage, outputDir)

	// 突变图表
	mutationDataSets := map[string]struct {
		Data  []float64
		Color string
	}{
		"批次总和 (batch_sum)":  {Data: mutationData, Color: "#FF6384"},
		"批次均值 (batch_mean)": {Data: mutationDataMean, Color: "#4BC0C0"},
	}
	g.generateCycleAnalysisChart(b, "3.3 平均突变随轮次变化", "平均突变随轮次变化", "mutation", posData, mutationDataSets, g.config.UseGoEcharts, g.config.EmbedImage, outputDir)

	// 插入图表
	insertionDataSets := map[string]struct {
		Data  []float64
		Color string
	}{
		"批次总和 (batch_sum)":  {Data: insertionData, Color: "#FF6384"},
		"批次均值 (batch_mean)": {Data: insertionDataMean, Color: "#4BC0C0"},
	}
	g.generateCycleAnalysisChart(b, "3.4 平均插入随轮次变化", "平均插入随轮次变化", "insertion", posData, insertionDataSets, g.config.UseGoEcharts, g.config.EmbedImage, outputDir)

	// 生成热力图
	// g.generateHeatmapSection(b, data, outputDir)
}

// generateHeatmapSection 生成热力图部分
func (g *Generator) generateHeatmapSection(b *strings.Builder, data *ReportData, outputDir string) {
	// 准备热力图数据
	if len(data.PositionStatsMean) > 0 {
		// 提取前20个位置的数据用于热力图
		maxPos := 20
		if len(data.PositionStatsMean) < maxPos {
			maxPos = len(data.PositionStatsMean)
		}

		heatmapData := make([][]float64, maxPos)
		for i := 0; i < maxPos; i++ {
			ps := data.PositionStatsMean[i]
			heatmapData[i] = []float64{ps.AvgYield, ps.AvgDeletion, ps.AvgMutation, ps.AvgInsertion}
		}

		g.generateHeatmapChart(b, "3.5 合成轮次热力图", "合成轮次热力图", "cycle", heatmapData, g.config.UseGoEcharts, g.config.EmbedImage, outputDir)
	}
}

// writeAppendixSection 写入附录部分
func (g *Generator) writeAppendixSection(b *strings.Builder, data *ReportData, batchID string) {
	b.WriteString("<h2>4. 附录</h2>\n")

	// 4.1 单孔单轮信息——收率
	b.WriteString(`<div class="section">` + "\n")
	b.WriteString("<h3>4.1 单孔单轮信息——收率</h3>\n")
	g.printWellPositionTableHTML(b, data, "yield", batchID)
	b.WriteString("</div>\n") // 结束section

	// 4.2 单孔单轮信息——缺失
	b.WriteString(`<div class="section" style="page-break-before: always;">` + "\n")
	b.WriteString("<h3>4.2 单孔单轮信息——缺失</h3>\n")
	g.printWellPositionTableHTML(b, data, "deletion", batchID)
	b.WriteString("</div>\n") // 结束section

	// 4.3 单孔单轮信息——插入
	b.WriteString(`<div class="section" style="page-break-before: always;">` + "\n")
	b.WriteString("<h3>4.3 单孔单轮信息——插入</h3>\n")
	g.printWellPositionTableHTML(b, data, "insertion", batchID)
	b.WriteString("</div>\n") // 结束section

	// 4.4 单孔单轮信息——突变
	b.WriteString(`<div class="section" style="page-break-before: always;">` + "\n")
	b.WriteString("<h3>4.4 单孔单轮信息——突变</h3>\n")
	g.printWellPositionTableHTML(b, data, "mutation", batchID)
	b.WriteString("</div>\n") // 结束section
}

// 常量和辅助函数
const (
	Rows = 12
	Cols = 8
)

// 列字母到索引的映射
var colIndex = map[string]int{
	"H": 0, "G": 1, "F": 2, "E": 3,
	"D": 4, "C": 5, "B": 6, "A": 7,
}
var colName = []string{
	"H", "G", "F", "E",
	"D", "C", "B", "A",
}

// 辅助函数
func (g *Generator) buildWellPlate(wells []*Well) [Rows][Cols]*Well {
	var plate [Rows][Cols]*Well
	for _, well := range wells {
		if well.Row >= 1 && well.Row <= Rows {
			if col, ok := colIndex[well.ColLetter]; ok && col >= 0 && col < Cols {
				plate[well.Row-1][col] = well
			}
		}
	}
	return plate
}

// 生成 batchID (合成日期+仪器号，仅保留字母数字)
func (g *Generator) makeBatchID(synthesisDate, instrumentID string) string {
	s := strings.ReplaceAll(synthesisDate, ".", "") + instrumentID
	var out strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// 从孔板提取非零数值列表（过滤零值？根据需求，收率可能为0，我们保留所有值）
func (g *Generator) extractValues(plate [Rows][Cols]*Well, field string) []float64 {
	var values []float64
	for i := 0; i < Rows; i++ {
		for j := 0; j < Cols; j++ {
			if well := plate[i][j]; well != nil {
				switch field {
				case "yield":
					values = append(values, well.Yield)
				case "deletion":
					values = append(values, well.Deletion)
				case "mutation":
					values = append(values, well.Mutation)
				case "insertion":
					values = append(values, well.Insertion)
				case "predicted_yield":
					values = append(values, well.PredictedYield)
				default:
					continue
				}
			}
		}
	}
	return values
}

func (g *Generator) extractFieldPlate(plate [Rows][Cols]*Well, field string) [Rows][Cols]interface{} {
	var result [Rows][Cols]interface{}
	for i := 0; i < Rows; i++ {
		for j := 0; j < Cols; j++ {
			if well := plate[i][j]; well != nil {
				switch field {
				case "name":
					result[i][j] = well.Name
				case "predicted_yield":
					result[i][j] = fmt.Sprintf("%.2f%%", well.Yield) // 这里假设Yield是预测收率
				case "yield":
					result[i][j] = fmt.Sprintf("%.2f%%", well.Yield)
				case "deletion":
					result[i][j] = fmt.Sprintf("%.2f%%", well.Deletion)
				case "mutation":
					result[i][j] = fmt.Sprintf("%.2f%%", well.Mutation)
				case "insertion":
					result[i][j] = fmt.Sprintf("%.2f%%", well.Insertion)
				default:
					result[i][j] = ""
				}
			} else {
				result[i][j] = ""
			}
		}
	}
	return result
}

// 格式化单元格（通用）
func (g *Generator) formatCell(cell interface{}) string {
	if cell == nil {
		return ""
	}
	switch v := cell.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.2f", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// printPlateTableHTML 打印板位表格
func (g *Generator) printPlateTableHTML(w io.Writer, title, subtitle, note, batchID string, plate [Rows][Cols]interface{}, overlapMatrix *[Rows][Cols]bool) {
	fmt.Fprint(w, `<div class="section">`+"\n")
	fmt.Fprintf(w, "<h3>%s</h3>\n", html.EscapeString(title))
	if subtitle != "" {
		fmt.Fprintf(w, "<p><em>%s</em></p>\n", html.EscapeString(subtitle))
	}

	// 表格样式：固定布局，单元格固定宽高，居中对齐
	fmt.Fprint(w, `<table border="1" cellpadding="4"  style=" table-layout: fixed; width: auto;; min-width: 800px;">`+"\n")
	fmt.Fprint(w, `<colgroup>`+"\n")
	fmt.Fprint(w, `<col style="width: 120px;">`+"\n") // 行号列宽度
	for range Cols {
		fmt.Fprintf(w, `<col style="width: 120px;">`+"\n") // 每列宽度120px
	}
	fmt.Fprint(w, "\n")
	fmt.Fprint(w, `</colgroup>`+"\n")

	// 表头行：第一个单元格为 batchID，其余为列字母
	fmt.Fprint(w, "<tr>")
	if batchID != "" {
		fmt.Fprintf(w, `<th style="height: 40px;">%s</th>`, html.EscapeString(batchID))
	} else {
		fmt.Fprint(w, `<th style="height: 40px;"></th>`)
	}
	for _, c := range colName {
		fmt.Fprintf(w, `<th style="height: 40px;">%s</th>`, c)
	}
	fmt.Fprint(w, "</tr>\n")

	// 数据行
	for i := range Rows {
		// 在第6行之后（即 i == 6）插入一个空行
		if i == 6 {
			fmt.Fprint(w, `<tr><td colspan="9" style="height: 20px; border: none;"></td></tr>`+"\n")
		}
		fmt.Fprintf(w, "<tr>")
		// 行号单元格
		fmt.Fprintf(w, `<th style="height: 30px;">%d</th>`, i+1)
		for j := 0; j < Cols; j++ {
			cell := g.formatCell(plate[i][j])
			// 判断是否需要标黄
			style := `style="height: 30px; text-align: center;"`
			if overlapMatrix != nil && overlapMatrix[i][j] {
				style = `style="height: 30px; text-align: center; background-color: yellow;"`
			}
			fmt.Fprintf(w, `<td %s>%s</td>`, style, html.EscapeString(cell))
		}
		fmt.Fprint(w, "</tr>\n")
	}
	fmt.Fprint(w, "</table>\n")
	if note != "" {
		fmt.Fprintf(w, "<p><small>注：%s</small></p>\n", html.EscapeString(note))
	}
	fmt.Fprint(w, "</div>\n") // 结束section
}

// printWellPositionTableHTML 生成单孔单轮信息表格
func (g *Generator) printWellPositionTableHTML(w io.Writer, data *ReportData, dataType, batchID string) {
	// 确定数据类型对应的字段和标题
	var dataField func(*PositionStats) float64
	switch dataType {
	case "yield":
		dataField = func(ps *PositionStats) float64 { return ps.AvgYield }
	case "deletion":
		dataField = func(ps *PositionStats) float64 { return ps.AvgDeletion }
	case "insertion":
		dataField = func(ps *PositionStats) float64 { return ps.AvgInsertion }
	case "mutation":
		dataField = func(ps *PositionStats) float64 { return ps.AvgMutation }
	default:
		return
	}

	// 收集所有位置
	positions := make(map[int]bool)
	for _, well := range data.Wells {
		for _, ps := range well.PositionStats {
			positions[ps.Pos] = true
		}
	}

	// 将位置转换为切片并排序
	var posSlice []int
	for pos := range positions {
		posSlice = append(posSlice, pos)
	}
	sort.Ints(posSlice)

	// 为每个位置生成一个表格
	for _, pos := range posSlice {
		// 表格样式：固定布局，单元格固定宽高，居中对齐
		fmt.Fprint(w, `<table border="1" cellpadding="4" cellspacing="0" style="border-collapse: collapse; table-layout: fixed; width: auto;; min-width: 800px;">`+"\n")
		fmt.Fprint(w, `<colgroup>`+"\n")
		fmt.Fprint(w, `<col style="width: 120px;">`+"\n") // 行号列宽度
		for range Cols {
			fmt.Fprintf(w, `<col style="width: 120px;">`+"\n") // 每列宽度120px
		}
		fmt.Fprint(w, `</colgroup>`+"\n")

		// 表头行：第一个单元格为标题，其余为列字母
		fmt.Fprint(w, "<tr>")
		fmt.Fprintf(w, `<th style="height: 40px;">%s<br/>%03d轮</th>`, batchID, pos)
		cols := []string{"H", "G", "F", "E", "D", "C", "B", "A"}
		for _, c := range cols {
			fmt.Fprintf(w, `<th style="height: 40px;">%s</th>`, c)
		}
		fmt.Fprint(w, "</tr>\n")

		// 准备板位数据
		var plate [Rows][Cols]interface{}
		for i := 0; i < Rows; i++ {
			for j := 0; j < Cols; j++ {
				plate[i][j] = ""
			}
		}

		// 填充板位数据
		for _, well := range data.Wells {
			// 查找该孔位在当前位置的数据
			for _, ps := range well.PositionStats {
				if ps.Pos == pos {
					// 计算行和列索引
					row := well.Row - 1
					col, ok := colIndex[well.ColLetter]
					if ok && row >= 0 && row < Rows && col >= 0 && col < Cols {
						// 格式化数据，保留2位小数
						plate[row][col] = fmt.Sprintf("%.2f%%", dataField(&ps))
					}
					break
				}
			}
		}

		// 数据行
		for i := 0; i < Rows; i++ {
			// 在第6行之后（即 i == 6）插入一个空行
			if i == 6 {
				fmt.Fprint(w, `<tr><td colspan="9" style="height: 20px; border: none;"></td></tr>`+"\n")
			}
			fmt.Fprintf(w, "<tr>")
			// 行号单元格
			fmt.Fprintf(w, `<th style="height: 30px;">%d</th>`, i+1)
			for j := 0; j < Cols; j++ {
				cell := g.formatCell(plate[i][j])
				fmt.Fprintf(w, `<td style="height: 30px; text-align: center;">%s</td>`, html.EscapeString(cell))
			}
			fmt.Fprint(w, "</tr>\n")
		}
		fmt.Fprint(w, "</table>\n")
		fmt.Fprint(w, `<br>`+"\n")
	}
}

// generateCycleAnalysisChart 生成合成轮次分析图表
func (g *Generator) generateCycleAnalysisChart(w io.Writer, sectionTitle, chartTitle, tag string, posData []int, dataSets map[string]struct {
	Data  []float64
	Color string
}, useGoEcharts, embedImage bool, outputDir string) {
	fmt.Fprint(w, `<div class="section">`+"\n")
	fmt.Fprintf(w, "<h3>%s</h3>\n", sectionTitle)
	if useGoEcharts {
		// 使用go-echarts生成图表
		chart := g.generateCycleAnalysisGoEcharts(chartTitle, posData, dataSets)
		fmt.Fprint(w, `<div class="chart-container">`+"\n")
		fmt.Fprint(w, chart)
		fmt.Fprint(w, `</div>`+"\n")
	} else {
		// 使用内置SVG生成器
		svg := g.generateCycleAnalysisSVG(chartTitle, posData, dataSets)
		if embedImage {
			fmt.Fprint(w, `<div class="chart-container">`+"\n")
			fmt.Fprint(w, svg)
			fmt.Fprint(w, `</div>`+"\n")
		} else {
			// 生成文件名
			fileName := "cycle_analysis_" + tag + ".svg"
			fullPath := filepath.Join(outputDir, fileName)
			fmt.Fprint(w, `<div class="chart-container">`+"\n")
			fmt.Fprintf(w, `<img src="./%s" alt="%s" style="width: 100%%;">`+"\n", fileName, chartTitle)
			fmt.Fprint(w, `</div>`+"\n")

			// 保存SVG到文件
			if err := os.WriteFile(fullPath, []byte(svg), 0644); err != nil {
				log.Printf("警告：保存SVG文件失败: %v", err)
			}
		}
	}
	fmt.Fprint(w, "</div>\n") // 结束section
}

// generateCycleAnalysisGoEcharts 使用go-echarts生成合成轮次分析图表
func (g *Generator) generateCycleAnalysisGoEcharts(title string, posData []int, dataSets map[string]struct {
	Data  []float64
	Color string
}) string {
	// 创建折线图
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: title}),
		charts.WithXAxisOpts(opts.XAxis{Name: "位置 (pos)"}),
		charts.WithYAxisOpts(
			opts.YAxis{
				Name:  "百分比 (%)",
				Scale: opts.Bool(true),
			},
		),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "bottom"}),
	)

	// 设置X轴数据
	line.SetXAxis(posData)

	// 添加数据系列
	for name, set := range dataSets {
		// 准备数据
		var items []opts.LineData
		for _, val := range set.Data {
			items = append(items, opts.LineData{Value: val})
		}

		// 添加数据系列
		line.AddSeries(name, items)
	}

	// 使用RenderSnippet生成图表部分
	snippet := line.Renderer.RenderSnippet()

	// 组合图表部分
	b := &strings.Builder{}
	b.WriteString(snippet.Element)
	b.WriteString(snippet.Script)

	return b.String()
}

// generateCycleAnalysisSVG 生成合成轮次分析的SVG图表
func (g *Generator) generateCycleAnalysisSVG(title string, posData []int, dataSets map[string]struct {
	Data  []float64
	Color string
}) string {
	// 图表尺寸
	width := 1000
	height := 400
	margin := 60

	// 计算数据范围
	maxY := 0.0
	minY := 100.0
	for _, set := range dataSets {
		for _, val := range set.Data {
			if val > maxY {
				maxY = val
			}
			if val < minY {
				minY = val
			}
		}
	}

	// 为了让图表更美观，在最大值和最小值上增加一些边距
	padding := (maxY - minY) * 0.05
	maxY += padding
	minY -= padding
	if minY < 0 {
		minY = 0
	}
	if maxY > 99 {
		maxY = 100
	}

	// 计算X轴范围
	minX := posData[0]
	maxX := posData[len(posData)-1]
	rangeX := maxX - minX
	rangeY := maxY - minY

	// 生成SVG
	b := &strings.Builder{}
	fmt.Fprintf(b, `<svg width="%d" height="%d" xmlns="http://www.w3.org/2000/svg">`, width, height)

	// 绘制背景
	fmt.Fprintf(b, `<rect width="%d" height="%d" fill="white"/>`, width, height)

	// 绘制标题
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="16" text-anchor="middle">%s</text>`, width/2, 20, title)

	// 绘制X轴
	fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="2"/>`, margin, height-margin, width-margin, height-margin)

	// 绘制Y轴
	fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="2"/>`, margin, margin, margin, height-margin)

	// 绘制X轴标签
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="12" text-anchor="middle">位置 (pos)</text>`, width/2, height-10)

	// 绘制Y轴标签
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="12" text-anchor="middle" transform="rotate(-90, %d, %d)">百分比 (%%)</text>`, 30, height/2, 30, height/2)

	// 绘制X轴刻度
	for i := 0; i <= 10; i++ {
		x := margin + i*(width-2*margin)/10
		val := minX + i*rangeX/10
		fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="1"/>`, x, height-margin, x, height-margin+5)
		fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="middle">%d</text>`, x, height-margin+15, int(val))
	}

	// 确定Y轴标签的小数位数
	precision := 0
	if rangeY < 5 {
		precision = 1
	} else if rangeY < 1 {
		precision = 2
	}

	// 绘制Y轴刻度
	for i := 0; i <= 10; i++ {
		y := height - margin - i*(height-2*margin)/10
		val := minY + float64(i)*rangeY/10
		fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="1"/>`, margin-5, y, margin, y)
		switch precision {
		case 0:
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.0f</text>`, margin-10, y+3, val)
		case 1:
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.1f</text>`, margin-10, y+3, val)
		default:
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.2f</text>`, margin-10, y+3, val)
		}
	}

	// 绘制数据线条
	for _, set := range dataSets {
		if len(set.Data) >= 2 {
			fmt.Fprintf(b, `<path d="`)
			for j, val := range set.Data {
				x := margin + (posData[j]-minX)*(width-2*margin)/rangeX
				y := height - margin - int((val-minY)*float64(height-2*margin)/rangeY)
				if j == 0 {
					fmt.Fprintf(b, "M %d %d", x, y)
				} else {
					fmt.Fprintf(b, " L %d %d", x, y)
				}
			}
			fmt.Fprintf(b, `" stroke="%s" stroke-width="2" fill="none"/>`, set.Color)
		}
	}

	// 绘制图例
	legendX := margin + 50
	legendY := 40
	i := 0
	for name, set := range dataSets {
		// 绘制颜色方块
		fmt.Fprintf(b, `<rect x="%d" y="%d" width="12" height="12" fill="%s"/>`, legendX, legendY+i*20, set.Color)
		// 绘制文字
		fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="12">%s</text>`, legendX+20, legendY+i*20+10, name)
		i++
	}

	fmt.Fprint(b, `</svg>`)
	return b.String()
}

// generateHeatmapChart 生成热力图
func (g *Generator) generateHeatmapChart(w io.Writer, sectionTitle, chartTitle, tag string, data [][]float64, useGoEcharts, embedImage bool, outputDir string) {
	fmt.Fprint(w, `<div class="section">`+"\n")
	fmt.Fprintf(w, "<h3>%s</h3>\n", sectionTitle)
	if useGoEcharts {
		// 使用go-echarts生成热力图
		heatmap := charts.NewHeatMap()
		heatmap.SetGlobalOptions(
			charts.WithTitleOpts(opts.Title{Title: chartTitle}),
			charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
			charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true)}),
		)

		// 准备数据
		var items []opts.HeatMapData
		for i, row := range data {
			for j, val := range row {
				items = append(items, opts.HeatMapData{Value: []interface{}{j, i, val}})
			}
		}

		heatmap.AddSeries("", items)

		// 生成图表
		snippet := heatmap.Renderer.RenderSnippet()
		b := &strings.Builder{}
		b.WriteString(snippet.Element)
		b.WriteString(snippet.Script)

		fmt.Fprint(w, `<div class="chart-container">`+"\n")
		fmt.Fprint(w, b.String())
		fmt.Fprint(w, `</div>`+"\n")
	} else {
		// 使用内置SVG生成器
		svg := g.generateHeatmapSVG(chartTitle, data)
		if embedImage {
			fmt.Fprint(w, `<div class="chart-container">`+"\n")
			fmt.Fprint(w, svg)
			fmt.Fprint(w, `</div>`+"\n")
		} else {
			// 生成文件名
			fileName := "heatmap_" + tag + ".svg"
			fullPath := filepath.Join(outputDir, fileName)
			fmt.Fprint(w, `<div class="chart-container">`+"\n")
			fmt.Fprintf(w, `<img src="./%s" alt="%s" style="width: 100%%;">`+"\n", fileName, chartTitle)
			fmt.Fprint(w, `</div>`+"\n")

			// 保存SVG到文件
			if err := os.WriteFile(fullPath, []byte(svg), 0644); err != nil {
				log.Printf("警告：保存SVG文件失败: %v", err)
			}
		}
	}
	fmt.Fprint(w, "</div>\n") // 结束section
}

// generateHeatmapSVG 生成热力图的SVG
func (g *Generator) generateHeatmapSVG(title string, data [][]float64) string {
	// 图表尺寸
	width := 1000
	height := 600
	margin := 80

	// 计算数据范围
	maxVal := 0.0
	minVal := 100.0
	for _, row := range data {
		for _, val := range row {
			if val > maxVal {
				maxVal = val
			}
			if val < minVal {
				minVal = val
			}
		}
	}

	// 生成SVG
	b := &strings.Builder{}
	fmt.Fprintf(b, `<svg width="%d" height="%d" xmlns="http://www.w3.org/2000/svg">`, width, height)

	// 绘制背景
	fmt.Fprintf(b, `<rect width="%d" height="%d" fill="white"/>`, width, height)

	// 绘制标题
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="16" text-anchor="middle">%s</text>`, width/2, 30, title)

	// 计算单元格大小
	rows := len(data)
	cols := 0
	if rows > 0 {
		cols = len(data[0])
	}
	cellWidth := (width - 2*margin) / cols
	cellHeight := (height - 2*margin) / rows

	// 绘制热力图
	for i, row := range data {
		for j, val := range row {
			// 计算颜色
			color := g.getHeatmapColor(val, minVal, maxVal)
			x := margin + j*cellWidth
			y := margin + i*cellHeight
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="#ccc" stroke-width="1"/>`, x, y, cellWidth, cellHeight, color)
			// 绘制数值
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="middle" fill="%s">%.2f</text>`, x+cellWidth/2, y+cellHeight/2+4, g.getTextColor(val, minVal, maxVal), val)
		}
	}

	fmt.Fprint(b, `</svg>`)
	return b.String()
}

// getHeatmapColor 根据值获取热力图颜色
func (g *Generator) getHeatmapColor(val, min, max float64) string {
	// 归一化值
	norm := (val - min) / (max - min)
	// 从蓝色到红色的渐变
	if norm < 0.25 {
		// 蓝色到青色
		return fmt.Sprintf("rgb(%d, %d, 255)", int(norm*4*255), 255)
	} else if norm < 0.5 {
		// 青色到绿色
		return fmt.Sprintf("rgb(0, %d, %d)", 255, int((0.5-norm)*4*255))
	} else if norm < 0.75 {
		// 绿色到黄色
		return fmt.Sprintf("rgb(%d, 255, 0)", int((norm-0.5)*4*255))
	} else {
		// 黄色到红色
		return fmt.Sprintf("rgb(255, %d, 0)", int((1-norm)*4*255))
	}
}

// getTextColor 根据背景色获取文字颜色
func (g *Generator) getTextColor(val, min, max float64) string {
	norm := (val - min) / (max - min)
	if norm > 0.5 {
		return "black"
	} else {
		return "white"
	}
}
