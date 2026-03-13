package main

// ------------------------------------------------------------
// 数据结构定义（与 JSON 一一对应）
// ------------------------------------------------------------

// ------------------------------------------------------------
// 数据结构（以孔为中心）
// ------------------------------------------------------------
type Well struct {
	Row            int     `json:"row"`                // 1-12
	ColLetter      string  `json:"col_letter"`         // H,G,F,E,D,C,B,A
	Name           string  `json:"name"`               // 引物名称
	IsOverlap      bool    `json:"is_overlap"`         // 是否重合序列
	PredictedYield float64 `json:"predicted_yield"`    // 预测收率 (%)
	Yield          float64 `json:"yield"`              // 实际收率 (%)
	Deletion       float64 `json:"deletion"`           // 缺失 (%)
	Mutation       float64 `json:"mutation"`           // 突变 (%)
	Insertion      float64 `json:"insertion"`          // 插入 (%)
	Sequence       string  `json:"sequence,omitempty"` // 序列
	Position       string  `json:"position,omitempty"` // 位置
}

// 96孔板数据：12行 x 8列，列顺序固定为 H,G,F,E,D,C,B,A
type WellPlate struct {
	Rows [12][8]interface{} `json:"rows"` // 可存放 string / float64 / nil
}

// 定义合成排板专用结构（带 Note 字段）
type PlateLayout struct {
	Title string    `json:"title"`
	Note  string    `json:"note"` // 注脚，如“标黄引物为重合序列”
	Plate WellPlate `json:"plate"`
}

// 带有平均值和标准差的板位数据
type PlateStats struct {
	Title  string    `json:"title"`
	AvgStd string    `json:"avg_std"` // 如 "48.78%±16.12%"
	Plate  WellPlate `json:"plate"`
}

// 错误统计条目
type ErrorStat struct {
	ErrorType string   `json:"error_type"` // 如 "平均收率"
	Reference *float64 `json:"reference"`  // 参考值，可以为 null
	Data      *float64 `json:"data"`       // 实际数据，可以为 null
}

// 基本信息
type BasicInfo struct {
	SynthesisDate       string `json:"synthesis_date"`
	InstrumentID        string `json:"instrument_id"`
	WellCount           int    `json:"well_count"`
	SynthesisLength     int    `json:"synthesis_length"`
	SynthesisProcessVer string `json:"synthesis_process_version"`
	SEC1ProcessVer      string `json:"sec1_process_version"`
	SequencingDate      string `json:"sequencing_date"`
}

// 概要统计
type SummaryStats struct {
	AvgYield              float64 `json:"avg_yield"`
	YieldStddev           float64 `json:"yield_stddev"`
	YieldMedian           float64 `json:"yield_median"`
	YieldQuartile         float64 `json:"yield_quartile"`
	YieldLt1Count         int     `json:"yield_lt1_count"`
	YieldLt5Count         int     `json:"yield_lt5_count"`
	PredictedDifficultSeq int     `json:"predicted_difficult_sequences"`
	OverlapSequences      int     `json:"overlap_sequences"`
}

// 概要部分
type Summary struct {
	BasicInfo  BasicInfo    `json:"basic_info"`
	Statistics SummaryStats `json:"statistics"`
	ErrorStats []ErrorStat  `json:"error_stats"`
}

// 板位分析部分
// 修改 PlateAnalysis 结构，将 PlateLayout 字段改为专用类型
type PlateAnalysis struct {
	PlateLayout    PlateLayout `json:"plate_layout"` // 修正为独立类型
	PredictedYield PlateStats  `json:"predicted_yield"`
	YieldStats     PlateStats  `json:"yield_stats"`
	DeletionStats  PlateStats  `json:"deletion_stats"`
	MutationStats  PlateStats  `json:"mutation_stats"`
	InsertionStats PlateStats  `json:"insertion_stats"`
	// 其他板位统计可按需添加
}

// 单轮数据（用于附录）
type CyclePlate struct {
	Cycle int                `json:"cycle"`
	Plate [12][8]interface{} `json:"plate"`
}

// 轮次分析（折线图部分，这里仅提供数据，报告只打印标题，后续可扩展）
type CycleAnalysis struct {
	AvgYieldByCycle     []float64 `json:"avg_yield_by_cycle"`
	AvgDeletionByCycle  []float64 `json:"avg_deletion_by_cycle"`
	AvgMutationByCycle  []float64 `json:"avg_mutation_by_cycle"`
	AvgInsertionByCycle []float64 `json:"avg_insertion_by_cycle"`
}

// 附录
type Appendix struct {
	YieldByWellCycle     []CyclePlate `json:"yield_by_well_cycle"`
	DeletionByWellCycle  []CyclePlate `json:"deletion_by_well_cycle"`
	InsertionByWellCycle []CyclePlate `json:"insertion_by_well_cycle"`
	MutationByWellCycle  []CyclePlate `json:"mutation_by_well_cycle"`
}

// 根结构
type ReportData struct {
	ReportTitle         string  `json:"report_title"`
	SynthesisDate       string  `json:"synthesis_date"`
	InstrumentID        string  `json:"instrument_id"`
	WellCount           int     `json:"well_count"`
	SynthesisLength     int     `json:"synthesis_length"`
	SynthesisProcessVer string  `json:"synthesis_process_version"`
	SEC1ProcessVer      string  `json:"sec1_process_version"`
	SequencingDate      string  `json:"sequencing_date"`
	Wells               []*Well `json:"wells"` // 所有孔的列表
	// 错误统计的参考值（仅用于输出参考列，数据由程序计算）
	ErrorStatsRef []ErrorStatRef `json:"error_stats_ref"`

	Summary Summary `json:"summary"`
	// PlateAnalysis PlateAnalysis `json:"plate_analysis"`
	CycleAnalysis CycleAnalysis `json:"cycle_analysis"`
	Appendix      Appendix      `json:"appendix"`
}

// 错误统计参考值条目
type ErrorStatRef struct {
	ErrorType string   `json:"error_type"`
	Reference *float64 `json:"reference"` // 可以为 null
}
