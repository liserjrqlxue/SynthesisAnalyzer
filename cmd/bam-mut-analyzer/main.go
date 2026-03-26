package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	stats "SynthesisAnalyzer/pkg/stats"
)

// Config 存放所有命令行参数及派生配置
type Config struct {
	InputDir         string
	OutputDir        string
	ExcelFile        string
	LogLevel         string
	HeadCut          int
	TailCut          int
	NMerSize         int
	MaxSubstitutions int
	MaxThreads       int
}

// parseFlags 解析命令行参数，返回配置对象
func parseFlags() *Config {
	cfg := &Config{}

	// 计算默认最大线程数: max(8, CPU个数/8)
	defaultMaxThreads := max(runtime.NumCPU()/8, 8)

	flag.StringVar(&cfg.InputDir, "d", "", "输入目录，包含样本子目录")
	flag.StringVar(&cfg.OutputDir, "o", "", "输出目录，默认输入目录/mutation_stats")
	flag.StringVar(&cfg.ExcelFile, "i", "", "可选参数：输入Excel文件，包含样本顺序")
	flag.IntVar(&cfg.HeadCut, "head", 27, "头切除长度")
	flag.IntVar(&cfg.TailCut, "tail", 20, "尾切除长度")
	flag.IntVar(&cfg.MaxSubstitutions, "max-sub", 5, "最大替换个数阈值，用于定义比对良好reads")
	flag.IntVar(&cfg.NMerSize, "n", 4, "N-mer 统计的 N 值（默认4，即统计5-mer准确率）")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "日志级别 (debug, info, warn, error)")
	flag.IntVar(&cfg.MaxThreads, "max-threads", defaultMaxThreads, "最大线程数，默认值为max(8, CPU个数/8)")

	flag.Parse()
	return cfg
}

// validateConfig 验证配置合法性，返回错误
func validateConfig(cfg *Config) error {
	if cfg.InputDir == "" {
		return fmt.Errorf("必需参数缺失，请使用 -d 指定输入目录")
	}
	info, err := os.Stat(cfg.InputDir)
	if err != nil {
		return fmt.Errorf("输入目录不存在: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("输入路径不是目录: %s", cfg.InputDir)
	}
	return nil
}

// setLogLevel 设置slog日志级别
func setupLogger(levelStr string) {
	var level slog.Level
	switch levelStr {
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

// run 执行核心业务逻辑，返回错误
func run(cfg *Config) error {
	// 初始化样本信息
	sampleInfo := stats.NewSampleInfo()
	sampleInfo.InputExcel = cfg.ExcelFile
	sampleInfo.InputDir = cfg.InputDir
	sampleInfo.HeadCuts = cfg.HeadCut
	sampleInfo.TailCuts = cfg.TailCut
	sampleInfo.NMerSize = cfg.NMerSize
	sampleInfo.MaxSubstitutions = cfg.MaxSubstitutions
	sampleInfo.MaxThreads = cfg.MaxThreads

	// 设置输出目录
	sampleInfo.OutputDir = cfg.OutputDir
	if sampleInfo.OutputDir == "" {
		sampleInfo.OutputDir = filepath.Join(sampleInfo.InputDir, "mutation_stats")
	}

	// 读取 Excel 样本顺序（如果提供）
	if cfg.ExcelFile != "" {
		if err := sampleInfo.ReadExcel(); err != nil {
			return fmt.Errorf("读取Excel文件失败: %w", err)
		}
		slog.Info("从Excel读取样本", "count", len(sampleInfo.Order))
	}

	// 查找所有BAM文件
	if err := sampleInfo.FindBAMFiles(); err != nil {
		return fmt.Errorf("查找BAM文件失败: %w", err)
	}
	slog.Info("找到BAM文件", "count", len(sampleInfo.Order))

	// 创建输出目录
	if err := os.MkdirAll(sampleInfo.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 统计处理
	mutationStats := stats.NewMutationStats()
	mutationStats.SampleInfo = sampleInfo
	if err := mutationStats.ProcessBAMFiles(); err != nil {
		return fmt.Errorf("处理BAM文件失败: %w", err)
	}

	slog.Info("开始生成统计文件...")
	mutationStats.SortSampleNames()
	mutationStats.MainWrite()
	mutationStats.MainPrint()

	return nil
}

func main() {
	// 解析命令行参数
	cfg := parseFlags()

	// 先设置日志，以便后续错误也能输出
	setupLogger(cfg.LogLevel)

	// 验证配置
	if err := validateConfig(cfg); err != nil {
		slog.Error("配置验证失败", "error", err)
		os.Exit(1)
	}

	// 执行核心业务逻辑
	if err := run(cfg); err != nil {
		slog.Error("运行失败", "error", err)
		os.Exit(1)
	}
}
