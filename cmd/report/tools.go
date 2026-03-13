package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

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
