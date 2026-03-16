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
// 主函数：读取JSON并生成报告
// ------------------------------------------------------------
func main() {
	inputFile := flag.String("i", "", "输入JSON文件路径")
	outputFile := flag.String("o", "", "输出报告文件路径（默认输出到stdout）")
	mutationStatsDir := flag.String("m", "", "mutation_stats目录路径（可选）")
	bomFile := flag.String("b", "", "BOM.xlsx文件路径（可选）")
	embedImage := flag.Bool("embed-image", false, "是否将图表以Base64编码嵌入HTML（默认false，使用外部图片文件）")
	useGoEcharts := flag.Bool("use-go-echarts", false, "是否使用go-echarts生成图表（默认false，使用内置SVG生成器）")
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

	// 如果提供了BOM.xlsx文件，读取并更新孔位信息
	if *bomFile != "" {
		// 读取BOM文件，更新孔位信息
		wellMap, err := ReadBOMFile(*bomFile)
		if err != nil {
			log.Printf("警告：读取BOM文件失败: %v", err)
		} else {
			// 更新孔位信息
			updateWellInfo(&report, wellMap)
		}
	}

	// 如果提供了mutation_stats目录，读取并更新数据
	if *mutationStatsDir != "" {
		// 读取子类型统计数据
		subtypeStats, err := ReadSubtypeStats(*mutationStatsDir)
		if err != nil {
			log.Printf("警告：读取子类型统计数据失败: %v", err)
		} else {
			// 更新错误统计
			updateErrorStats(&report, subtypeStats)
		}

		// 读取收率和错误统计数据
		yieldStats, deletionStats, mutationStats, insertionStats, err := ReadYieldStats(*mutationStatsDir)
		if err != nil {
			log.Printf("警告：读取收率和错误统计数据失败: %v", err)
		} else {
			// 更新收率和错误数据
			updateYieldData(&report, yieldStats, deletionStats, mutationStats, insertionStats)
		}

		// 读取split_summary.txt中的总处理reads数
		barcodeReads, err := ReadSplitSummary(*mutationStatsDir)
		if err != nil {
			log.Printf("警告：读取split_summary.txt失败: %v", err)
		} else {
			// 更新BarcodeReads字段
			report.BarcodeReads = barcodeReads
			log.Printf("从split_summary.txt更新BarcodeReads: %d", barcodeReads)
		}

		// 读取位置统计数据
		positionStats, err := ReadPositionStats(*mutationStatsDir)
		if err != nil {
			log.Printf("警告：读取位置统计数据失败: %v", err)
		} else {
			log.Printf("成功读取位置统计数据，共%d个位置", len(positionStats))
			// 将positionStats存储在report结构中，以便在生成报告时使用
			report.PositionStats = positionStats
		}
	}

	// 生成报告文本
	htmlReport := GenerateHTMLReport(&report, *embedImage, *useGoEcharts)

	// 输出
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(htmlReport), 0644); err != nil {
			log.Fatalf("写入文件失败: %v", err)
		}
	} else {
		fmt.Print(htmlReport)
	}
}

// updateErrorStats 更新错误统计数据
func updateErrorStats(report *ReportData, subtypeStats map[string]float64) {
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
func updateYieldData(report *ReportData, yieldStats map[string]float64, deletionStats map[string]float64, mutationStats map[string]float64, insertionStats map[string]float64) {
	// 计算收率统计值
	avgYield, stdYield, medYield, q1Yield, lt1Count, lt5Count := CalculateYieldStatistics(yieldStats)

	// 更新概要统计
	report.Summary.Statistics.AvgYield = avgYield
	report.Summary.Statistics.YieldStddev = stdYield
	report.Summary.Statistics.YieldMedian = medYield
	report.Summary.Statistics.YieldQuartile = q1Yield
	report.Summary.Statistics.YieldLt1Count = lt1Count
	report.Summary.Statistics.YieldLt5Count = lt5Count
	fmt.Printf("%+v\n", report.Summary.Statistics)

	// 更新每个孔的收率和错误数据（如果能匹配到样本名称）
	for _, well := range report.Wells {
		if yield, ok := yieldStats[well.Name]; ok {
			well.Yield = yield
		}
		if deletion, ok := deletionStats[well.Name]; ok {
			well.Deletion = deletion
		}
		if mutation, ok := mutationStats[well.Name]; ok {
			well.Mutation = mutation
		}
		if insertion, ok := insertionStats[well.Name]; ok {
			well.Insertion = insertion
		}
	}
}
