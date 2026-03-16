package main

import (
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// ------------------------------------------------------------
// HTML 表格打印函数（带 batchID 左上角）
// ------------------------------------------------------------
func printPlateTableHTML(w io.Writer, title, subtitle, note, batchID string, plate [Rows][Cols]interface{}, overlapMatrix *[Rows][Cols]bool) {
	fmt.Fprintf(w, "<h3>%s</h3>\n", html.EscapeString(title))
	if subtitle != "" {
		fmt.Fprintf(w, "<p><em>%s</em></p>\n", html.EscapeString(subtitle))
	}

	// 表格样式：固定布局，单元格固定宽高，居中对齐
	fmt.Fprint(w, `<table border="1" cellpadding="4" cellspacing="0" style="border-collapse: collapse; table-layout: fixed; width: auto;; min-width: 800px;">`+"\n")
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
	cols := strings.Split(ColHeaders, "\t")
	for _, c := range cols {
		fmt.Fprintf(w, `<th style="height: 40px;">%s</th>`, c)
	}
	fmt.Fprint(w, "</tr>\n")

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
			cell := formatCell(plate[i][j])
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
}

// generateCycleAnalysisGoEcharts 使用go-echarts生成合成轮次分析图表
func generateCycleAnalysisGoEcharts(title string, posData []int, data []float64, color string) string {
	// 计算数据范围
	maxY := 0.0
	minY := 100.0
	for _, val := range data {
		if val > maxY {
			maxY = val
		}
		if val < minY {
			minY = val
		}
	}

	// 为了让图表更美观，在最大值和最小值上增加一些边距
	padding := (maxY - minY) * 0.05
	maxY += padding
	minY -= padding
	if minY < 0 {
		minY = 0
	}

	// 对于平均合成收率，确保Y轴范围合理
	if title == "平均合成收率随轮次变化" {
		minY = 90
		if maxY < 95 {
			maxY = 95
		}
		if maxY > 100 {
			maxY = 100
		}
	}

	// 创建折线图
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: title}),
		charts.WithXAxisOpts(opts.XAxis{Name: "位置 (pos)"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "百分比 (%)", Min: minY, Max: maxY}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "bottom"}),
	)

	// 设置X轴数据
	line.SetXAxis(posData)

	// 准备数据
	var items []opts.LineData
	for _, val := range data {
		items = append(items, opts.LineData{Value: val})
	}

	// 添加数据系列
	line.AddSeries(title, items)

	// 设置系列样式
	line.SetSeriesOptions(
		charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(false)}),
		charts.WithItemStyleOpts(opts.ItemStyle{Color: color}),
	)

	// 使用RenderSnippet生成图表部分
	snippet := line.Renderer.RenderSnippet()

	// 组合图表部分
	b := &strings.Builder{}
	b.WriteString(snippet.Element)
	b.WriteString(snippet.Script)

	return b.String()
}

// generateCycleAnalysisSVG 生成合成轮次分析的SVG图表
func generateCycleAnalysisSVG(title string, posData []int, data []float64, color string) string {
	// 图表尺寸
	width := 1000
	height := 400
	margin := 60

	// 计算数据范围
	maxY := 0.0
	minY := 100.0
	for _, val := range data {
		if val > maxY {
			maxY = val
		}
		if val < minY {
			minY = val
		}
	}

	// 为了让图表更美观，在最大值和最小值上增加一些边距
	padding := (maxY - minY) * 0.05
	// if padding < 1 {
	// padding = 1
	// }
	maxY += padding
	minY -= padding
	if minY < 0 {
		minY = 0
	}

	// 对于平均合成收率，确保Y轴范围合理
	if title == "平均合成收率随轮次变化" {
		// minY = 90
		if maxY < 95 {
			maxY = 95
		}
		if maxY > 100 {
			maxY = 100
		}
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
	fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="12" text-anchor="middle" transform="rotate(-90, %d, %d)">百分比 (%)</text>`, 30, height/2, 30, height/2)

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
		if precision == 0 {
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.0f</text>`, margin-10, y+3, val)
		} else if precision == 1 {
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.1f</text>`, margin-10, y+3, val)
		} else {
			fmt.Fprintf(b, `<text x="%d" y="%d" font-family="Arial" font-size="10" text-anchor="end">%.2f</text>`, margin-10, y+3, val)
		}
	}

	// 绘制数据线条
	if len(data) >= 2 {
		fmt.Fprintf(b, `<path d="`)
		for j, val := range data {
			x := margin + (posData[j]-minX)*(width-2*margin)/rangeX
			y := height - margin - int((val-minY)*float64(height-2*margin)/rangeY)
			if j == 0 {
				fmt.Fprintf(b, "M %d %d", x, y)
			} else {
				fmt.Fprintf(b, " L %d %d", x, y)
			}
		}
		fmt.Fprintf(b, `" stroke="%s" stroke-width="2" fill="none"/>`, color)
	}

	fmt.Fprint(b, `</svg>`)
	return b.String()
}

// ------------------------------------------------------------
// 生成完整 HTML 报告
// ------------------------------------------------------------
func GenerateHTMLReport(data *ReportData, embedImage bool, useGoEcharts bool, outputFile string) string {
	b := &strings.Builder{}

	// 获取输出目录
	outputDir := "."
	if outputFile != "" {
		outputDir = filepath.Dir(outputFile)
	}

	// HTML 头部
	fmt.Fprint(b, `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>`+html.EscapeString(data.ReportTitle)+`</title>
<script src="https://go-echarts.github.io/go-echarts-assets/assets/echarts.min.js"></script>
<style>
body { font-family: sans-serif; margin: 2em; }
h1 { color: #333; }
h2 { border-bottom: 1px solid #ccc; padding-bottom: 0.3em; }
table { margin: 1em 0; }
th { background-color: #f2f2f2; }
.chart-container { width: 100%; max-width: 1000px; margin: 0 auto; margin-top: 30px; display: flex; justify-content: center; align-items: center; }
.chart-item { width: 900px; height: 500px; margin: auto; }
</style>
</head>
<body>
`)
	fmt.Fprintf(b, "<h1>%s</h1>\n", html.EscapeString(data.ReportTitle))

	// 构建二维孔索引
	plate := buildWellPlate(data.Wells)
	batchID := makeBatchID(data.SynthesisDate, data.InstrumentID)

	// --------------------------------------------------------
	// 1. Summary
	// --------------------------------------------------------
	fmt.Fprint(b, "<h2>1. Summary</h2>\n")

	// 基本信息
	fmt.Fprint(b, "<h3>基本信息</h3>\n")
	// 表格样式：固定布局，单元格固定宽高，居中对齐

	fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;; min-width: 800px;">`+"\n")
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprintf(b, "<tr><td>合成日期</td><td>%s</td></tr>\n", data.SynthesisDate)
	fmt.Fprintf(b, "<tr><td>仪器号</td><td>%s</td></tr>\n", data.InstrumentID)
	fmt.Fprintf(b, "<tr><td>合成孔数</td><td>%d</td></tr>\n", data.WellCount)
	fmt.Fprintf(b, "<tr><td>合成长度</td><td>%dnt</td></tr>\n", data.SynthesisLength)
	fmt.Fprintf(b, "<tr><td>合成工艺版本</td><td>%s</td></tr>\n", data.SynthesisProcessVer)
	fmt.Fprintf(b, "<tr><td>SEC1工艺版本</td><td>%s</td></tr>\n", data.SEC1ProcessVer)
	fmt.Fprintf(b, "<tr><td>测序日期</td><td>%s</td></tr>\n", data.SequencingDate)
	fmt.Fprintf(b, "<tr><td>Barcode数据量（reads）</td><td>%d</td></tr>\n", data.BarcodeReads)
	// fmt.Fprint(b, "</table>\n")

	// 统计概要 - 使用从mutation_stats获取的更新后数据
	stats := data.Summary.Statistics
	// 提取收率值，用于板位统计
	yieldVals := extractValues(plate, "yield")

	// 建立参考值映射
	refMap := make(map[string]*float64)
	for _, ref := range data.ErrorStatsRef {
		refMap[ref.ErrorType] = ref.Reference
	}

	// fmt.Fprint(b, "<h3>合成信息</h3>\n")
	// fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;; min-width: 800px;">`+"\n")
	// fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	// fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度

	// 平均收率 - 根据参考值是否存在来决定输出格式
	if refVal, ok := refMap["平均收率"]; ok && refVal != nil && *refVal > 0 {
		fmt.Fprintf(b, "<tr><td>平均收率</td><td>%.2f%% (参考值%.2f%%)</td></tr>\n", stats.AvgYield, *refVal)
	} else {
		fmt.Fprintf(b, "<tr><td>平均收率</td><td>%.2f%%</td></tr>\n", stats.AvgYield)
	}

	fmt.Fprintf(b, "<tr><td>收率标准差</td><td>%.2f%%</td></tr>\n", stats.YieldStddev)
	fmt.Fprintf(b, "<tr><td>收率中位数</td><td>%.2f%%</td></tr>\n", stats.YieldMedian)
	fmt.Fprintf(b, "<tr><td>收率四分位数</td><td>%.2f%%</td></tr>\n", stats.YieldQuartile)
	fmt.Fprintf(b, "<tr><td>收率&lt;1%%片段个数</td><td>%d</td></tr>\n", stats.YieldLt1Count)
	fmt.Fprintf(b, "<tr><td>收率&lt;5%%片段个数</td><td>%d</td></tr>\n", stats.YieldLt5Count)
	fmt.Fprintf(b, "<tr><td>预测难度序列</td><td>%d</td></tr>\n", stats.PredictedDifficultSeq)
	fmt.Fprintf(b, "<tr><td>重合序列</td><td>%d</td></tr>\n", stats.OverlapSequences)
	fmt.Fprint(b, "</table>\n")
	fmt.Fprint(b, "<p><small>*注：预测难度序列需确定阈值</small></p>\n")

	// 合成错误统计
	fmt.Fprint(b, "<h3>合成错误统计</h3>\n")
	fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto; min-width: 800px;">`+"\n")
	fmt.Fprint(b, `<colgroup>`+"\n")                  // 列标题
	fmt.Fprint(b, `<col style="width: 140px;">`+"\n") // 错误类型-EN列宽度
	fmt.Fprint(b, `<col style="width: 230px;">`+"\n") // 错误类型-CN列宽度
	fmt.Fprint(b, `<col style="width: 120px;">`+"\n") // 示例列宽度
	// fmt.Fprint(b, `</colgroup>`+"\n")                 //
	// fmt.Fprint(b, `<colgroup>`+"\n")                  // 列值
	fmt.Fprint(b, `<col style="width: 120px;">`+"\n") // 参考值列宽度
	fmt.Fprint(b, `<col style="width: 120px;">`+"\n") // 数据列宽度
	fmt.Fprint(b, `</colgroup>`+"\n")                 //
	fmt.Fprint(b, "<tr><th>错误类型-EN</th><th>错误类型-CN</th><th>示例</th><th>参考值</th><th>数据</th></tr>\n")

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
		fmt.Fprintf(b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			errorInfo.EN, errorInfo.CN, errorInfo.Example, refStr, dataStr)
	}
	fmt.Fprint(b, "</table>\n")

	// --------------------------------------------------------
	// 2. 合成板位分析
	// --------------------------------------------------------
	fmt.Fprint(b, "<h2>2. 合成板位分析</h2>\n")

	// 构建重合矩阵（仅在合成排板时需要）
	var overlapMatrix [Rows][Cols]bool
	for i := 0; i < Rows; i++ {
		for j := 0; j < Cols; j++ {
			w := plate[i][j]
			if w != nil && w.IsOverlap {
				overlapMatrix[i][j] = true
			}
		}
	}

	// 2.1 合成排板
	namePlate := extractFieldPlate(plate, "name")
	printPlateTableHTML(b, "2.1 合成排板", "", "标黄引物为重合序列", batchID, namePlate, &overlapMatrix)

	// 2.2 预测合成收率
	predVals := extractValues(plate, "predicted_yield")
	meanPred, stdPred := meanStdDev(predVals)
	predTitle := fmt.Sprintf("2.2 预测合成收率 (%.2f%%±%.2f%%)", meanPred, stdPred)
	predPlate := extractFieldPlate(plate, "predicted_yield")
	printPlateTableHTML(b, predTitle, "", "", batchID, predPlate, nil)

	// 2.3 收率板位统计
	meanYield, stdYield2 := meanStdDev(yieldVals)
	yieldTitle := fmt.Sprintf("2.3 收率板位统计 (%.2f%%±%.2f%%)", meanYield, stdYield2)
	yieldPlate := extractFieldPlate(plate, "yield")
	printPlateTableHTML(b, yieldTitle, "", "", batchID, yieldPlate, nil)

	// 2.4 缺失板位统计
	delVals := extractValues(plate, "deletion")
	meanDel, stdDel := meanStdDev(delVals)
	delTitle := fmt.Sprintf("2.4 缺失板位统计 (%.2f%%±%.2f%%)", meanDel, stdDel)
	delPlate := extractFieldPlate(plate, "deletion")
	printPlateTableHTML(b, delTitle, "", "", batchID, delPlate, nil)

	// 2.5 突变板位统计
	mutVals := extractValues(plate, "mutation")
	meanMut, stdMut := meanStdDev(mutVals)
	mutTitle := fmt.Sprintf("2.5 突变板位统计 (%.2f%%±%.2f%%)", meanMut, stdMut)
	mutPlate := extractFieldPlate(plate, "mutation")
	printPlateTableHTML(b, mutTitle, "", "", batchID, mutPlate, nil)

	// 2.6 插入板位统计
	insVals := extractValues(plate, "insertion")
	meanIns, stdIns := meanStdDev(insVals)
	insTitle := fmt.Sprintf("2.6 插入板位统计 (%.2f%%±%.2f%%)", meanIns, stdIns)
	insPlate := extractFieldPlate(plate, "insertion")
	printPlateTableHTML(b, insTitle, "", "", batchID, insPlate, nil)

	// --------------------------------------------------------
	// 3. 合成轮次分析
	// --------------------------------------------------------
	fmt.Fprint(b, "<h2>3. 合成轮次分析</h2>\n")

	// 检查是否有位置统计数据
	if len(data.PositionStats) > 0 {
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

		// 3.1 平均合成收率随轮次变化
		fmt.Fprint(b, "<h3>3.1 平均合成收率随轮次变化</h3>\n")
		if useGoEcharts {
			// 使用go-echarts生成图表
			yieldChart := generateCycleAnalysisGoEcharts("平均合成收率随轮次变化", posData, yieldData, "#4BC0C0")
			fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
			fmt.Fprint(b, yieldChart)
			fmt.Fprint(b, `</div>`+"\n")
		} else {
			// 使用内置SVG生成器
			yieldSVG := generateCycleAnalysisSVG("平均合成收率随轮次变化", posData, yieldData, "#4BC0C0")
			if embedImage {
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprint(b, yieldSVG)
				fmt.Fprint(b, `</div>`+"\n")
			} else {
				yieldPath := "cycle_analysis_yield.svg"
				fullYieldPath := filepath.Join(outputDir, yieldPath)
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprintf(b, `<img src="%s" alt="平均合成收率随轮次变化" style="width: 100%;">`+"\n", yieldPath)
				fmt.Fprint(b, `</div>`+"\n")

				// 保存SVG到文件
				if err := os.WriteFile(fullYieldPath, []byte(yieldSVG), 0644); err != nil {
					log.Printf("警告：保存SVG文件失败: %v", err)
				}
			}
		}

		// 3.2 平均缺失随轮次变化
		fmt.Fprint(b, "<h3>3.2 平均缺失随轮次变化</h3>\n")
		if useGoEcharts {
			// 使用go-echarts生成图表
			deletionChart := generateCycleAnalysisGoEcharts("平均缺失随轮次变化", posData, deletionData, "#FF6384")
			fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
			fmt.Fprint(b, deletionChart)
			fmt.Fprint(b, `</div>`+"\n")
		} else {
			// 使用内置SVG生成器
			deletionSVG := generateCycleAnalysisSVG("平均缺失随轮次变化", posData, deletionData, "#FF6384")
			if embedImage {
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprint(b, deletionSVG)
				fmt.Fprint(b, `</div>`+"\n")
			} else {
				deletionPath := "cycle_analysis_deletion.svg"
				fullDeletionPath := filepath.Join(outputDir, deletionPath)
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprintf(b, `<img src="%s" alt="平均缺失随轮次变化" style="width: 100%;">`+"\n", deletionPath)
				fmt.Fprint(b, `</div>`+"\n")

				// 保存SVG到文件
				if err := os.WriteFile(fullDeletionPath, []byte(deletionSVG), 0644); err != nil {
					log.Printf("警告：保存SVG文件失败: %v", err)
				}
			}
		}

		// 3.3 平均突变随轮次变化
		fmt.Fprint(b, "<h3>3.3 平均突变随轮次变化</h3>\n")
		if useGoEcharts {
			// 使用go-echarts生成图表
			mutationChart := generateCycleAnalysisGoEcharts("平均突变随轮次变化", posData, mutationData, "#36A2EB")
			fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
			fmt.Fprint(b, mutationChart)
			fmt.Fprint(b, `</div>`+"\n")
		} else {
			// 使用内置SVG生成器
			mutationSVG := generateCycleAnalysisSVG("平均突变随轮次变化", posData, mutationData, "#36A2EB")
			if embedImage {
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprint(b, mutationSVG)
				fmt.Fprint(b, `</div>`+"\n")
			} else {
				mutationPath := "cycle_analysis_mutation.svg"
				fullMutationPath := filepath.Join(outputDir, mutationPath)
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprintf(b, `<img src="%s" alt="平均突变随轮次变化" style="width: 100%%;">`+"\n", mutationPath)
				fmt.Fprint(b, `</div>`+"\n")

				// 保存SVG到文件
				if err := os.WriteFile(fullMutationPath, []byte(mutationSVG), 0644); err != nil {
					log.Printf("警告：保存SVG文件失败: %v", err)
				}
			}
		}

		// 3.4 平均插入随轮次变化
		fmt.Fprint(b, "<h3>3.4 平均插入随轮次变化</h3>\n")
		if useGoEcharts {
			// 使用go-echarts生成图表
			insertionChart := generateCycleAnalysisGoEcharts("平均插入随轮次变化", posData, insertionData, "#FFCE56")
			fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
			fmt.Fprint(b, insertionChart)
			fmt.Fprint(b, `</div>`+"\n")
		} else {
			// 使用内置SVG生成器
			insertionSVG := generateCycleAnalysisSVG("平均插入随轮次变化", posData, insertionData, "#FFCE56")
			if embedImage {
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprint(b, insertionSVG)
				fmt.Fprint(b, `</div>`+"\n")
			} else {
				insertionPath := "cycle_analysis_insertion.svg"
				fullInsertionPath := filepath.Join(outputDir, insertionPath)
				fmt.Fprint(b, `<div style="width: 100%; max-width: 1000px; margin: 0 auto;">`+"\n")
				fmt.Fprintf(b, `<img src="%s" alt="平均插入随轮次变化" style="width: 100%;">`+"\n", insertionPath)
				fmt.Fprint(b, `</div>`+"\n")

				// 保存SVG到文件
				if err := os.WriteFile(fullInsertionPath, []byte(insertionSVG), 0644); err != nil {
					log.Printf("警告：保存SVG文件失败: %v", err)
				}
			}
		}
	} else {
		fmt.Fprint(b, `<p>未找到位置统计数据，无法生成合成轮次分析图表。</p>`+"\n")
	}

	// --------------------------------------------------------
	// 4. 附录（占位）
	// --------------------------------------------------------
	fmt.Fprint(b, "<h2>4. 附录</h2>\n")
	fmt.Fprint(b, "<p>4.1 单孔单轮信息——收率</p>\n")
	fmt.Fprint(b, "<p>4.2 单孔单轮信息——缺失</p>\n")
	fmt.Fprint(b, "<p>4.3 单孔单轮信息——插入</p>\n")
	fmt.Fprint(b, "<p>4.4 单孔单轮信息——突变</p>\n")
	fmt.Fprint(b, "<p>4.5 …</p>\n")

	fmt.Fprint(b, "</body>\n</html>")
	return b.String()
}
