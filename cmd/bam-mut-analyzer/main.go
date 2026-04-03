package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"SynthesisAnalyzer/pkg/cfg"
	"SynthesisAnalyzer/pkg/stats"
)

// parseFlags 解析命令行参数，返回配置对象
func parseFlags() *cfg.Config {
	config := &cfg.Config{}

	// 计算默认最大线程数: max(8, CPU个数/8)
	defaultMaxThreads := max(runtime.NumCPU()/8, 8)

	flag.StringVar(&config.InputDir, "d", "", "输入目录，包含样本子目录")
	flag.StringVar(&config.InputSheet, "s", "Sheet1", "输入Sheet名称，默认Sheet1")
	flag.StringVar(&config.OutputDir, "o", "", "输出目录，默认输入目录/mutation_stats")
	flag.StringVar(&config.ExcelFile, "i", "", "可选参数：输入Excel文件，包含样本顺序")
	flag.StringVar(&config.SampleNameSuffix, "suffix-col", "", "可选参数：样品名称后缀列，若指定则将该列值拼接到样品名称后")
	flag.IntVar(&config.HeadCut, "head", 27, "头切除长度")
	flag.IntVar(&config.TailCut, "tail", 20, "尾切除长度")
	flag.IntVar(&config.MaxSubstitutions, "max-sub", 5, "最大替换个数阈值，用于定义比对良好reads")
	flag.IntVar(&config.NMerSize, "n", 4, "N-mer 统计的 N 值（默认4，即统计5-mer准确率）")
	flag.StringVar(&config.LogLevel, "log-level", "info", "日志级别 (debug, info, warn, error)")
	flag.IntVar(&config.Threads, "max-threads", defaultMaxThreads, "最大线程数，默认值为max(8, CPU个数/8)")

	flag.Parse()

	if config.OutputDir == "" {
		config.OutputDir = filepath.Join(config.InputDir, cfg.DefaultMutationStatsDir)
	}

	return config
}

// validateConfig 验证配置合法性，返回错误
func validateConfig(cfg *cfg.Config) error {
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

// run 执行核心业务逻辑，返回错误
func run(config *cfg.Config) error {
	// 初始化样本信息
	batchInfo := cfg.NewBatchInfo()
	batchInfo.Config = config

	// 读取 Excel 样本顺序（如果提供）
	if config.ExcelFile != "" {
		if err := batchInfo.ReadExcel(); err != nil {
			return fmt.Errorf("读取Excel文件失败: %w", err)
		}
		slog.Info("从Excel读取样本", "count", len(batchInfo.SampleList))
	}

	// 查找所有BAM文件
	if err := batchInfo.FindBAMFiles(); err != nil {
		return fmt.Errorf("查找BAM文件失败: %w", err)
	}
	slog.Info("找到BAM文件", "count", len(batchInfo.SampleList))

	// 创建输出目录
	if err := os.MkdirAll(batchInfo.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 统计处理
	mutationStats := stats.NewMutationStats()
	mutationStats.BatchInfo = batchInfo
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
	cfg.SetLogLevel()

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
