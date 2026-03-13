package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// ------------------------------------------------------------
// 全局常量
// ------------------------------------------------------------
const (
	ColHeaders = "H\tG\tF\tE\tD\tC\tB\tA" // 列标题，制表符分隔便于对齐
	Rows       = 12
	Cols       = 8
)

// 列字母到索引的映射
var colIndex = map[string]int{
	"H": 0, "G": 1, "F": 2, "E": 3,
	"D": 4, "C": 5, "B": 6, "A": 7,
}

// ------------------------------------------------------------
// 打印一个完整的96孔板表格
//   - title: 表格标题（如“2.1 合成排板”）
//   - subtitle: 副标题（如平均值±标准差，可为空）
//   - note: 注脚（如“标黄引物为重合序列”，可为空）
//   - plate: WellPlate 数据
//
// ------------------------------------------------------------
func printPlateTable(w io.Writer, title, subtitle, note string, plate [Rows][Cols]interface{}) {
	fmt.Fprintf(w, "\n%s\n", title)
	if subtitle != "" {
		fmt.Fprintf(w, "%s\n", subtitle)
	}
	// 表头
	fmt.Fprintf(w, "\t%s\n", ColHeaders)
	// 数据行
	for i := 0; i < Rows; i++ {
		rowCells := make([]string, Cols)
		for j := 0; j < Cols; j++ {
			rowCells[j] = formatCell(plate[i][j])
		}
		fmt.Fprintf(w, "%d\t%s\n", i+1, strings.Join(rowCells, "\t"))
	}
	if note != "" {
		fmt.Fprintf(w, "*注：%s\n", note)
	}
}

// ------------------------------------------------------------
// 打印概要基本信息（两列表格）
// ------------------------------------------------------------
func printBasicInfo(w io.Writer, info BasicInfo) {
	fmt.Fprintln(w, "\n基本信息")
	fmt.Fprintf(w, "合成日期\t%s\n", info.SynthesisDate)
	fmt.Fprintf(w, "仪器号\t%s\n", info.InstrumentID)
	fmt.Fprintf(w, "合成孔数\t%d\n", info.WellCount)
	fmt.Fprintf(w, "合成长度\t%dnt\n", info.SynthesisLength)
	fmt.Fprintf(w, "合成工艺版本\t%s\n", info.SynthesisProcessVer)
	fmt.Fprintf(w, "SEC1工艺版本\t%s\n", info.SEC1ProcessVer)
	fmt.Fprintf(w, "测序日期\t%s\n", info.SequencingDate)
}

// ------------------------------------------------------------
// 打印概要统计（键值对）
// ------------------------------------------------------------
func printSummaryStats(w io.Writer, stats SummaryStats) {
	fmt.Fprintf(w, "平均收率\t%.2f%% (参考值)\n", stats.AvgYield)
	fmt.Fprintf(w, "收率标准差\t%.2f%%\n", stats.YieldStddev)
	fmt.Fprintf(w, "收率中位数\t%.2f%%\n", stats.YieldMedian)
	fmt.Fprintf(w, "收率四分位数\t%.2f%%\n", stats.YieldQuartile)
	fmt.Fprintf(w, "收率<1%%片段个数\t%d\n", stats.YieldLt1Count)
	fmt.Fprintf(w, "收率<5%%片段个数\t%d\n", stats.YieldLt5Count)
	fmt.Fprintf(w, "预测难度序列\t%d\n", stats.PredictedDifficultSeq)
	fmt.Fprintf(w, "重合序列\t%d\n", stats.OverlapSequences)
	fmt.Fprintln(w, "*注：预测难度序列需确定阈值")
}

// ------------------------------------------------------------
// 打印合成错误统计表
// ------------------------------------------------------------
func printErrorStats(w io.Writer, stats []ErrorStat) {
	fmt.Fprintln(w, "\n合成错误统计")
	fmt.Fprintln(w, "错误类型\t参考值\t数据")
	for _, s := range stats {
		ref := ""
		if s.Reference != nil {
			ref = fmt.Sprintf("%.2f", *s.Reference)
		}
		dat := ""
		if s.Data != nil {
			dat = fmt.Sprintf("%.2f", *s.Data)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.ErrorType, ref, dat)
	}
}

// ------------------------------------------------------------
// 生成完整报告
// ------------------------------------------------------------
func GenerateReport(data *ReportData) string {
	b := &strings.Builder{}

	// 标题
	fmt.Fprintf(b, "%s\n", data.ReportTitle)
	fmt.Fprintln(b, strings.Repeat("=", len(data.ReportTitle)))

	// 构建二维孔索引
	plate := buildWellPlate(data.Wells)

	// 生成批次标识（合成日期+仪器号）
	batchID := fmt.Sprintf("%s%s", strings.ReplaceAll(data.Summary.BasicInfo.SynthesisDate, ".", ""), data.Summary.BasicInfo.InstrumentID)

	// 移除可能的中文字符，保留数字字母
	batchID = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return r
		}
		return -1
	}, batchID)

	// 统计概要（从孔数据计算）
	var stats = data.Summary.Statistics

	yieldVals := extractValues(plate, "yield")
	stats.AvgYield, stats.YieldStddev = meanStdDev(yieldVals)
	stats.YieldMedian = median(yieldVals)
	stats.YieldQuartile = quartile(yieldVals)
	for _, w := range data.Wells {
		if w.Yield < 1 {
			stats.YieldLt1Count++
		}
		if w.Yield < 5 {
			stats.YieldLt5Count++
		}
	}
	stats.PredictedDifficultSeq = 0 // 需要单独字段或逻辑，暂时设为0或从JSON其他位置读取
	for _, w := range data.Wells {
		if w.IsOverlap {
			stats.OverlapSequences++
		}
	}

	// ------------------------------------------------------------
	// 1. Summary
	// ------------------------------------------------------------
	fmt.Fprintln(b, "\n1.\tSummary")
	printBasicInfo(b, data.Summary.BasicInfo)
	printSummaryStats(b, data.Summary.Statistics)
	// printErrorStats(b, data.Summary.ErrorStats)

	// 合成错误统计（动态计算并输出）
	fmt.Fprintln(b, "\n合成错误统计")
	fmt.Fprintln(b, "错误类型\t参考值\t数据")

	// 预定义错误类型列表及其计算方式
	errorStats := []struct {
		Name string
		Calc func() *float64
	}{
		{"平均收率", func() *float64 { v := stats.AvgYield; return &v }},
		{"收率标准差", func() *float64 { v := stats.YieldStddev; return &v }},
		{"收率中位数", func() *float64 { v := stats.YieldMedian; return &v }},
		{"收率四分位数", func() *float64 { v := stats.YieldQuartile; return &v }},
		{"收率<1%片段个数", func() *float64 { v := float64(stats.YieldLt1Count); return &v }},
		{"收率<5%片段个数", func() *float64 { v := float64(stats.YieldLt5Count); return &v }},
		{"缺失", func() *float64 { v, _ := meanStdDev(extractValues(plate, "deletion")); return &v }},
		{"突变", func() *float64 { v, _ := meanStdDev(extractValues(plate, "mutation")); return &v }},
		{"插入", func() *float64 { v, _ := meanStdDev(extractValues(plate, "insertion")); return &v }},
		{"插入+缺失", func() *float64 {
			// 如果有单独字段则用，否则用缺失平均值+插入平均值
			var sum float64
			cnt := 0
			for _, w := range data.Wells {
				if w.InsertionDeletion > 0 {
					sum += w.InsertionDeletion
					cnt++
				}
			}
			if cnt > 0 {
				v := sum / float64(cnt)
				return &v
			}
			delAvg, _ := meanStdDev(extractValues(plate, "deletion"))
			insAvg, _ := meanStdDev(extractValues(plate, "insertion"))
			v := delAvg + insAvg
			return &v
		}},
		{"缺失≥2 nt", func() *float64 {
			var sum float64
			cnt := 0
			for _, w := range data.Wells {
				if w.DeletionGt2 > 0 {
					sum += w.DeletionGt2
					cnt++
				}
			}
			if cnt > 0 {
				v := sum / float64(cnt)
				return &v
			}
			return nil
		}},
		{"其他突变", func() *float64 {
			var sum float64
			cnt := 0
			for _, w := range data.Wells {
				if w.OtherMutation > 0 {
					sum += w.OtherMutation
					cnt++
				}
			}
			if cnt > 0 {
				v := sum / float64(cnt)
				return &v
			}
			return nil
		}},
	}
	// 建立参考值映射
	refMap := make(map[string]*float64)
	for _, ref := range data.ErrorStatsRef {
		refMap[ref.ErrorType] = ref.Reference
	}

	for _, es := range errorStats {
		dataVal := es.Calc()
		refVal := refMap[es.Name]
		refStr := ""
		if refVal != nil {
			refStr = fmt.Sprintf("%.2f", *refVal)
		}
		dataStr := ""
		if dataVal != nil {
			dataStr = fmt.Sprintf("%.2f", *dataVal)
		}
		fmt.Fprintf(b, "%s\t%s\t%s\n", es.Name, refStr, dataStr)
	}

	// ------------------------------------------------------------
	// 2. 合成板位分析
	// ------------------------------------------------------------
	fmt.Fprintln(b, "\n2.\t合成板位分析")

	// 2.1 合成排板
	layoutPlate := extractFieldPlate(plate, "name")
	printPlateTable(b, "2.1 合成排板", "", "标黄引物为重合序列", layoutPlate)

	// 2.2 预测合成收率
	predVals := extractValues(plate, "predicted_yield")
	meanPred, stdPred := meanStdDev(predVals)
	predTitle := fmt.Sprintf("2.2 预测合成收率 (%.2f%%±%.2f%%)", meanPred, stdPred)
	predPlate := extractFieldPlate(plate, "predicted_yield")
	printPlateTable(b, predTitle, "", "", predPlate)

	// 2.3 收率板位统计
	meanYield, stdYield2 := meanStdDev(yieldVals) // 复用之前计算
	yieldTitle := fmt.Sprintf("2.3 收率板位统计 (%.2f%%±%.2f%%)", meanYield, stdYield2)
	yieldPlate := extractFieldPlate(plate, "yield")
	printPlateTable(b, yieldTitle, "", "", yieldPlate)

	// 2.4 缺失板位统计
	delVals := extractValues(plate, "deletion")
	meanDel, stdDel := meanStdDev(delVals)
	delTitle := fmt.Sprintf("2.4 缺失板位统计 (%.2f%%±%.2f%%)", meanDel, stdDel)
	delPlate := extractFieldPlate(plate, "deletion")
	printPlateTable(b, delTitle, "", "", delPlate)

	// 2.5 突变板位统计
	mutVals := extractValues(plate, "mutation")
	meanMut, stdMut := meanStdDev(mutVals)
	mutTitle := fmt.Sprintf("2.5 突变板位统计 (%.2f%%±%.2f%%)", meanMut, stdMut)
	mutPlate := extractFieldPlate(plate, "mutation")
	printPlateTable(b, mutTitle, "", "", mutPlate)

	// 2.6 插入板位统计
	insVals := extractValues(plate, "insertion")
	meanIns, stdIns := meanStdDev(insVals)
	insTitle := fmt.Sprintf("2.6 插入板位统计 (%.2f%%±%.2f%%)", meanIns, stdIns)
	insPlate := extractFieldPlate(plate, "insertion")
	printPlateTable(b, insTitle, "", "", insPlate)

	// 2.7 … 可根据需要扩展

	// 3. 合成轮次分析1
	// ------------------------------------------------------------
	// 3. 合成轮次分析（保留框架，可扩展）
	// ------------------------------------------------------------
	fmt.Fprintln(b, "\n3.\t合成轮次分析")
	// 折线图部分，按题目要求只输出标题，数据可后续扩展
	fmt.Fprintln(b, "3.1 平均合成收率随轮次变化(折线图)")
	// 这里可以打印数据表代替折线图，例如：
	if len(data.CycleAnalysis.AvgYieldByCycle) > 0 {
		fmt.Fprintln(b, "轮次\t平均收率(%)")
		for i, v := range data.CycleAnalysis.AvgYieldByCycle {
			fmt.Fprintf(b, "%d\t%.2f\n", i+1, v)
		}
	}
	fmt.Fprintln(b, "3.2 平均缺失随轮次变化")
	fmt.Fprintln(b, "3.3 平均突变随轮次变化")
	fmt.Fprintln(b, "3.4 平均插入随轮次变化")
	fmt.Fprintln(b, "3.5 …")

	// 4. 附录
	fmt.Fprintln(b, "\n4\t附录")
	// 4.1 单孔单轮信息——收率
	fmt.Fprintln(b, "4.1 单孔单轮信息——收率")
	for _, cp := range data.Appendix.YieldByWellCycle {
		title := fmt.Sprintf("%03d轮", cp.Cycle)
		printPlateTable(b, title, "", "", cp.Plate)
	}
	// 4.2-4.5 类似，可循环处理
	fmt.Fprintln(b, "4.2 单孔单轮信息——缺失")
	for _, cp := range data.Appendix.DeletionByWellCycle {
		title := fmt.Sprintf("%03d轮", cp.Cycle)
		printPlateTable(b, title, "", "", cp.Plate)
	}
	fmt.Fprintln(b, "4.3 单孔单轮信息——插入")
	for _, cp := range data.Appendix.InsertionByWellCycle {
		title := fmt.Sprintf("%03d轮", cp.Cycle)
		printPlateTable(b, title, "", "", cp.Plate)
	}
	fmt.Fprintln(b, "4.4 单孔单轮信息——突变")
	for _, cp := range data.Appendix.MutationByWellCycle {
		title := fmt.Sprintf("%03d轮", cp.Cycle)
		printPlateTable(b, title, "", "", cp.Plate)
	}
	fmt.Fprintln(b, "4.5 …")

	return b.String()
}

// ------------------------------------------------------------
// 主函数：读取JSON并生成报告
// ------------------------------------------------------------
func main() {
	inputFile := flag.String("i", "", "输入JSON文件路径")
	outputFile := flag.String("o", "", "输出报告文件路径（默认输出到stdout）")
	flag.Parse()

	if *inputFile == "" {
		log.Fatal("请使用 -i 指定输入JSON文件")
	}

	// 读取JSON
	jsonData, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("读取文件失败: %v", err)
	}

	var report ReportData
	if err := json.Unmarshal(jsonData, &report); err != nil {
		log.Fatalf("解析JSON失败: %v", err)
	}

	// 生成报告文本
	// reportText := GenerateReport(&report)
	htmlReport := GenerateHTMLReport(&report)

	// 输出
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(htmlReport), 0644); err != nil {
			log.Fatalf("写入文件失败: %v", err)
		}
	} else {
		fmt.Print(htmlReport)
	}
}
