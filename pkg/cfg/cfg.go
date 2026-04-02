package cfg

import (
	"log/slog"
	"os"
)

// Config 存放所有命令行参数及派生配置
type Config struct {
	// input
	ExcelFile        string // sample info excel file
	InputSheet       string // input sheet name
	SampleNameSuffix string // 样品名称后缀列
	FastqDir         string
	InputDir         string // 与OutputDir相同

	OutputDir string

	Threads int

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
	LogLevel         string
	HeadCut          int
	TailCut          int
	NMerSize         int
	MaxSubstitutions int

	// report 相关配置
	ReportConfig
	InputFile        string
	Prefix           string // 输出文件前缀
	MutationStatsDir string
	BomFile          string
	BatchID          string // 手动指定BatchID
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
