package cfg

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

var (
	DefaultMutationStatsDir = "mutation_stats"
)

// 比对配置
type AlignmentConfig struct {
	UseMinimap2    bool    // 使用minimap2而不是BWA
	AlignerThreads int     // 比对线程数
	MapQThreshold  int     // 比对质量阈值
	MinIdentity    float64 // 最小identity百分比
	SkipAlignment  bool    // 是否跳过比对步骤
	KeepSamFiles   bool    // 是否保留SAM文件
	AnalysisOnly   bool    // 仅分析已有的BAM文件
}

// 报告配置
type ReportConfig struct {
	ConfigFile string
	Template   string

	EmbedImage      bool
	UseGoEcharts    bool
	UseLocalEcharts bool   // 是否使用本地echarts资源
	EchartsPath     string // echarts资源路径
}

// Config 存放所有命令行参数及派生配置
type Config struct {
	// input
	ExcelFile        string // 输入Excel文件路径
	InputSheet       string // 输入Sheet名称
	SampleNameSuffix string // 样品名称后缀列
	FastqDir         string

	InputDir         string // bam-mut-analyzer输入目录路径
	OutputDir        string // bam-mut-analyzer输出目录路径,默认InputDir/mutation_stats
	MutationStatsDir string
	LogLevel         string
	Threads          int

	UseRC bool // 是否使用反向互补

	// 匹配参数
	SearchWindow int // 搜索窗口大小（从头、从尾搜索的距离）
	Quality      int // fastp质量阈值
	MergeLen     int // 合并后长度
	// 模糊匹配参数
	AllowMismatch  int // 允许的错配数（0表示完全匹配）
	MatchThreshold int // 匹配分数阈值

	// 运行选项
	SkipExisting  bool // 是否跳过已存在的文件
	Compression   bool // 是否压缩输出
	CompressLevel int  // 压缩级别 1-9，默认6
	CleanupTemp   bool // 是否清理临时文件

	// 输出选项
	OutputMode string // "full"或"target-only"

	// 比对相关配置
	Alignment AlignmentConfig

	// 污染检测
	ContaminationDetection bool // 是否启用交叉污染检测

	// --overlap_len_require
	// the minimum length to detect overlapped region of PE reads. This will affect overlap analysis based PE merge, adapter trimming and correction. 30 by default. (int [=30])
	OverlapLenRequire int

	// bam-mut-analyzer 相关配置
	HeadCut          int // 头切除长度
	TailCut          int // 尾切除长度
	NMerSize         int // n-mer大小
	MaxSubstitutions int // 最大错配次数

	// report 相关配置
	ReportConfig
	InputFile string
	Prefix    string // 输出文件前缀
	BomFile   string
	BatchID   string // 手动指定BatchID
}

func (cfg *Config) SetLogLevel() {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// 创建新的logger配置
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))

	// 设置全局logger
	slog.SetDefault(logger)
}

func (cfg *Config) LoadInputExcel() ([]*Sample, error) {
	if cfg.ExcelFile == "" {
		return nil, fmt.Errorf("-i required!")
	}
	slog.Info("Loading input Excel file", "file", cfg.ExcelFile)
	// 读取Excel文件
	samples, err := readExcel(cfg.ExcelFile, cfg.InputSheet, cfg.SampleNameSuffix)
	if err != nil {
		return nil, fmt.Errorf("Failed to load Excel file: %v", err)
	}

	return samples, nil
}

func readExcel(excelFile, sheetName, sampleNameSuffix string) ([]*Sample, error) {
	var samples []*Sample
	f, err := excelize.OpenFile(excelFile)
	if err != nil {
		return samples, fmt.Errorf("Failed to open Excel file: %v", err)
	}
	defer f.Close()

	// 读取工作表数据
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return samples, fmt.Errorf("Failed to read Excel sheet: %v", err)
	}
	if len(rows) < 2 {
		return samples, fmt.Errorf("Excel sheet %s is empty", sheetName)
	}

	// 构建表头映射
	headerMap := make(map[string]int)
	for col, header := range rows[0] {
		headerMap[header] = col
	}

	// 检查必需列
	requiredCols := []string{"样品名称", "靶标序列", "合成序列", "后靶标", "路径-R1", "路径-R2"}
	for _, col := range requiredCols {
		if _, ok := headerMap[col]; !ok {
			return samples, fmt.Errorf("缺少必需的列: %s:[%+v]", col, headerMap)
		}
	}

	// 检查后缀列
	var suffixColIndex int = -1
	if sampleNameSuffix != "" {
		if idx, ok := headerMap[sampleNameSuffix]; ok {
			suffixColIndex = idx
		} else {
			return samples, fmt.Errorf("未找到指定的后缀列: %s", sampleNameSuffix)
		}
	}

	// 读取数据行
	seenSampleNames := make(map[string]bool)
	for rowIdx, row := range rows {
		if rowIdx == 0 {
			continue // 跳过表头
		}

		// 确保行有足够的列
		if len(row) < len(rows[0]) {
			continue // 跳过不完整的行
		}

		sampleName := row[headerMap["样品名称"]]
		targetSeq := strings.ToUpper(row[headerMap["靶标序列"]])
		synthSeq := strings.ToUpper(row[headerMap["合成序列"]])
		postSeq := strings.ToUpper(row[headerMap["后靶标"]])
		r1Path := row[headerMap["路径-R1"]]
		r2Path := row[headerMap["路径-R2"]]

		// 检查必需字段
		if sampleName == "" {
			slog.Warn("样品名称为空，跳过\n", "row", rowIdx+1)
			continue // 跳过空行
		}

		// 拼接后缀列到样品名称
		if sampleNameSuffix != "" && suffixColIndex != -1 {
			if len(row) > suffixColIndex {
				suffix := strings.TrimSpace(row[suffixColIndex])
				if suffix != "" {
					sampleName = sampleName + "." + suffix
				}
			}
		}

		// 检查样品名称是否重复
		if seenSampleNames[sampleName] {
			return nil, fmt.Errorf("第 %d 行样品名称重复: %s", rowIdx+1, sampleName)
		}
		seenSampleNames[sampleName] = true

		fullSeq := strings.ToUpper(targetSeq + synthSeq + postSeq)

		sample := &Sample{
			Name:          sampleName,
			TargetSeq:     targetSeq,
			SynthesisSeq:  synthSeq,
			PostTargetSeq: postSeq,
			R1Path:        r1Path,
			R2Path:        r2Path,

			FullReference: fullSeq,
			RefLength:     len(fullSeq),
			HeadCut:       len(targetSeq), // 头切除长度 = 靶标序列长度
			TailCut:       len(postSeq),   // 尾切除长度 = 后靶标长度

		}

		samples = append(samples, sample)
		slog.Debug("读取样品", "name", sample.Name, "R1", filepath.Base(sample.R1Path), "R2", filepath.Base(sample.R2Path))
	}
	return samples, nil
}
