package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	// "compress/gzip"

	"SynthesisAnalyzer/pkg/cfg"
	"SynthesisAnalyzer/pkg/splitter"
)

var (
	excelFile = flag.String(
		"i",
		"",
		"<Excel文件>",
	)
	outputDir = flag.String(
		"o",
		"",
		"[输出目录]",
	)
	fastqDir = flag.String(
		"fq",
		"",
		"[Fastq目录]",
	)
	suffixCol = flag.String(
		"suffix-col",
		"",
		"可选参数：样品名称后缀列，若指定则将该列值拼接到样品名称后",
	)
	overlap = flag.Int(
		"m",
		30,
		"the minimum length to detect overlapped region of PE reads. This will affect overlap analysis based PE merge, adapter trimming and correction. 3",
	)
	contaminationDetection = flag.Bool(
		"contamination-detection",
		false,
		"启用交叉污染检测功能",
	)
	logLevel = flag.String(
		"log-level",
		"info",
		"日志级别: debug, info, warn, error",
	)
)

// 处理fastq目录
func processFastqDir(fastqDir, excelFile string) (string, error) {
	if fastqDir != "" {
		return fastqDir, nil
	}

	// 如果-fq未定义，查找-i对应目录内的*path.txt(兼容大写字符)
	excelDir := filepath.Dir(excelFile)
	files, err := filepath.Glob(filepath.Join(excelDir, "*[Pp][Aa][Tt][Hh].txt"))
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("No path.txt file found in the Excel directory")
	}

	// 读取path.txt文件内容
	pathFile := files[0]
	content, err := os.ReadFile(pathFile)
	if err != nil {
		return "", fmt.Errorf("Failed to read path.txt: %v", err)
	}

	fqBatch := strings.TrimSpace(string(content))
	if fqBatch == "" {
		return "", fmt.Errorf("Empty content in path.txt")
	}

	// 判断模式
	if matched, _ := regexp.MatchString(`^FT\d+$`, fqBatch); matched {
		// G99模式
		return fmt.Sprintf("/data2/wangyaoshen/Sequencing_data/G99/R21007100240139/%s/L01", fqBatch), nil
	} else if strings.HasPrefix(fqBatch, "oss://novo-medical-customer-tj/") {
		// Novo模式
		parts := strings.Split(fqBatch, "/")
		if len(parts) < 4 {
			return "", fmt.Errorf("Invalid Novo path format")
		}
		// 提取最后一个目录
		lastDir := parts[len(parts)-1]
		// 提取CYB编号 (parts[3] after splitting "oss://novo-medical-customer-tj/CYB24030020/...")
		cyb := parts[3]
		return fmt.Sprintf("/data2/wangyaoshen/novo-medical-customer-tj/%s/%s/Rawdata", cyb, lastDir), nil
	} else {
		return "", fmt.Errorf("Unsupported fqBatch format")
	}
}

func main() {
	flag.Parse()
	if *excelFile == "" {
		flag.Usage()
		log.Fatalln("-i required!")
	}

	// 处理输出目录
	output := *outputDir
	if output == "" {
		// 如果-o未定义，使用-i 切掉".xlsx"作为输出目录
		baseName := filepath.Base(*excelFile)
		if before, ok := strings.CutSuffix(baseName, ".xlsx"); ok {
			output = before
		} else {
			output = baseName
		}
	}

	// 处理fastq目录
	fastq, err := processFastqDir(*fastqDir, *excelFile)
	if err != nil {
		log.Fatalf("处理fastq目录失败: %v", err)
	}

	// 创建配置
	config := &cfg.Config{
		LogLevel:         *logLevel,
		ExcelFile:        *excelFile,
		OutputDir:        output,
		FastqDir:         fastq,
		Threads:          runtime.NumCPU(),
		SampleNameSuffix: *suffixCol,
		SearchWindow:     200, // 从头/尾搜索50bp
		Quality:          20,
		MergeLen:         80,

		UseRC:         true, // 启用反向互补匹配
		SkipExisting:  true, // 默认跳过已存在文件
		Compression:   true, // 默认启用压缩
		CompressLevel: 6,    // 默认压缩级别
		CleanupTemp:   true, // 不保留临时文件

		AllowMismatch:  0,             // 允许2个错配
		MatchThreshold: 30,            // 匹配分数阈值
		OutputMode:     "target-only", // 只输出靶标间序列

		Alignment: cfg.AlignmentConfig{
			UseMinimap2:    true,
			AlignerThreads: 16,
			MapQThreshold:  10,
			MinIdentity:    0.90,
			SkipAlignment:  false,
			KeepSamFiles:   false,
			AnalysisOnly:   false,
		},

		ContaminationDetection: *contaminationDetection,
		OverlapLenRequire:      *overlap,
	}

	// 设置日志级别
	config.SetLogLevel()

	// 处理可选参数
	for i := 4; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--skip-alignment":
			config.Alignment.SkipAlignment = true
		case "--analysis-only":
			config.Alignment.AnalysisOnly = true
		case "--keep-bam":
			config.Alignment.KeepSamFiles = true
		case "--threads":
			if i+1 < len(os.Args) {
				threads, err := strconv.Atoi(os.Args[i+1])
				if err == nil && threads > 0 {
					config.Threads = threads
				}
				i++
			}
		}
	}

	// 创建处理器
	splitter := splitter.NewEnhancedSplitter(config)

	// 运行处理流程
	if err := splitter.RunWithAlignment(); err != nil {
		fmt.Printf("处理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("处理完成!")
}

func printUsage() {
	fmt.Println(`FASTQ拆分与比对分析系统 v2.0

用法:
  fastq_analyzer <Excel文件> [输出目录] [Fastq目录] [选项]

示例:
  fastq_analyzer samples.xlsx ./results
  fastq_analyzer samples.xlsx ./output ./fastq --skip-alignment --threads 32

选项:
  --skip-alignment    跳过比对步骤（仅拆分）
  --analysis-only    仅分析已有的BAM文件
  --keep-bam         保留BAM文件（默认清理）
  --threads N        设置线程数
  --contamination-detection  启用交叉污染检测功能

输入Excel格式:
  Sheet1必须包含以下列：
    - 样品名称
    - 靶标序列
    - 合成序列
    - 后靶标
    - 路径-R1
    - 路径-R2

依赖软件:
  - fastp: 用于PE reads合并
  - minimap2: 用于序列比对
  - samtools: 用于BAM文件处理
  - R: 用于统计绘图（可选）`)
}
