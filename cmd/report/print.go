package main

import (
	"fmt"
	"html"
	"io"
	"strings"
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
	fmt.Fprint(w, `<table border="1" cellpadding="4" cellspacing="0" style="border-collapse: collapse; table-layout: fixed; width: auto;">`+"\n")
	fmt.Fprint(w, `<colgroup><col style="width: 120px;"></colgroup>`) // 行号列宽度
	for i := 0; i < Cols; i++ {
		fmt.Fprintf(w, `<col style="width: 120px;">`) // 每列宽度80px
	}
	fmt.Fprint(w, "\n")

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

// ------------------------------------------------------------
// 生成完整 HTML 报告
// ------------------------------------------------------------
func GenerateHTMLReport(data *ReportData) string {
	b := &strings.Builder{}

	// HTML 头部
	fmt.Fprint(b, `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>`+html.EscapeString(data.ReportTitle)+`</title>
<style>
body { font-family: sans-serif; margin: 2em; }
h1 { color: #333; }
h2 { border-bottom: 1px solid #ccc; padding-bottom: 0.3em; }
table { margin: 1em 0; }
th { background-color: #f2f2f2; }
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

	fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;">`+"\n")
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprintf(b, "<tr><td>合成日期</td><td>%s</td></tr>\n", data.SynthesisDate)
	fmt.Fprintf(b, "<tr><td>仪器号</td><td>%s</td></tr>\n", data.InstrumentID)
	fmt.Fprintf(b, "<tr><td>合成孔数</td><td>%d</td></tr>\n", data.WellCount)
	fmt.Fprintf(b, "<tr><td>合成长度</td><td>%dnt</td></tr>\n", data.SynthesisLength)
	fmt.Fprintf(b, "<tr><td>合成工艺版本</td><td>%s</td></tr>\n", data.SynthesisProcessVer)
	fmt.Fprintf(b, "<tr><td>SEC1工艺版本</td><td>%s</td></tr>\n", data.SEC1ProcessVer)
	fmt.Fprintf(b, "<tr><td>测序日期</td><td>%s</td></tr>\n", data.SequencingDate)
	fmt.Fprint(b, "</table>\n")

	// 统计概要 - 使用从mutation_stats获取的更新后数据
	stats := data.Summary.Statistics
	// 提取收率值，用于板位统计
	yieldVals := extractValues(plate, "yield")

	// 建立参考值映射
	refMap := make(map[string]*float64)
	for _, ref := range data.ErrorStatsRef {
		refMap[ref.ErrorType] = ref.Reference
	}

	fmt.Fprint(b, "<h3>统计信息</h3>\n")
	fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;">`+"\n")
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度

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
	fmt.Fprint(b, `<table border='1' cellpadding='4' style="table-layout: fixed; width: auto;">`+"\n")
	fmt.Fprint(b, `<colgroup><col style="width: 240px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, `<colgroup><col style="width: 120px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, `<colgroup><col style="width: 120px;"></colgroup>`) // 行号列宽度
	fmt.Fprint(b, "<tr><th>错误类型</th><th>参考值</th><th>数据</th></tr>\n")

	for _, es := range data.Summary.ErrorStats {
		// 获取错误类型的显示名称
		errorTypeName := es.ErrorType
		if mappedName, ok := subtypeMapping[es.ErrorType]; ok {
			errorTypeName = mappedName
		}

		refVal := refMap[es.ErrorType]
		refStr := ""
		if refVal != nil {
			refStr = fmt.Sprintf("%.2f", *refVal)
		}
		dataStr := ""
		if es.Data != nil {
			dataStr = fmt.Sprintf("%.2f", *es.Data)
		}
		fmt.Fprintf(b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			errorTypeName, refStr, dataStr)
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
	// 3. 合成轮次分析（占位）
	// --------------------------------------------------------
	fmt.Fprint(b, "<h2>3. 合成轮次分析</h2>\n")
	fmt.Fprint(b, "<p>3.1 平均合成收率随轮次变化(折线图)</p>\n")
	fmt.Fprint(b, "<p>3.2 平均缺失随轮次变化</p>\n")
	fmt.Fprint(b, "<p>3.3 平均突变随轮次变化</p>\n")
	fmt.Fprint(b, "<p>3.4 平均插入随轮次变化</p>\n")
	fmt.Fprint(b, "<p>3.5 …</p>\n")

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
