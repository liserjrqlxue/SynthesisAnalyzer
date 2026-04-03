package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	// "compress/gzip"

	"SynthesisAnalyzer/pkg/cfg"
	"SynthesisAnalyzer/pkg/splitter"
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
	// 解析命令行参数
	config := parseFlags()

	// 先设置日志，以便后续错误也能输出
	config.SetLogLevel()

	// 后处理逻辑：OutputDir 默认值等
	if config.OutputDir == "" {
		// 如果-o未定义，使用-i 切掉".xlsx"作为输出目录
		baseName := filepath.Base(config.ExcelFile)
		if before, ok := strings.CutSuffix(baseName, ".xlsx"); ok {
			config.OutputDir = before
		} else {
			config.OutputDir = baseName
		}
	}

	// 处理fastq目录
	fastq, err := processFastqDir(config.FastqDir, config.ExcelFile)
	if err != nil {
		log.Fatalf("处理fastq目录失败: %v", err)
	}
	config.FastqDir = fastq

	// 补充固定默认值
	config.UseRC = true // 启用反向互补匹配
	config.Quality = 20
	config.MergeLen = 80
	config.SearchWindow = 200         // 从头/尾搜索200bp
	config.SkipExisting = true        // 默认跳过已存在文件
	config.CompressLevel = 6          // 默认压缩级别
	config.CleanupTemp = true         // 不保留临时文件
	config.AllowMismatch = 0          // 允许2个错配
	config.MatchThreshold = 30        // 匹配分数阈值
	config.OutputMode = "target-only" // 只输出靶标间序列
	config.Alignment = cfg.AlignmentConfig{
		UseMinimap2:   true,
		MapQThreshold: 10,
		MinIdentity:   0.90,
	}

	// 创建处理器
	splitter := splitter.NewEnhancedSplitter(config)

	// 运行处理流程
	if err := splitter.RunAll(); err != nil {
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
  fastq_analyzer samples.xlsx ./output ./fastq 

选项:
  -contamination-detection  启用交叉污染检测功能

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

// parseFlags 解析命令行参数，返回配置对象
func parseFlags() *cfg.Config {
	config := &cfg.Config{}

	flag.StringVar(&config.ExcelFile, "i", "", "<Excel文件> (必需)")
	flag.StringVar(&config.InputSheet, "s", "Sheet1", "输入Sheet名称，默认Sheet1")
	flag.StringVar(&config.OutputDir, "o", "", "[输出目录] 默认为Excel文件名去.xlsx")
	flag.StringVar(&config.FastqDir, "fq", "", "[Fastq目录]")
	flag.StringVar(&config.SampleNameSuffix, "suffix-col", "", "可选：样品名称后缀列，若指定则将该列值拼接到样品名称后")

	// the minimum length to detect overlapped region of PE reads. This will affect overlap analysis based PE merge, adapter trimming and correction. 3
	flag.IntVar(&config.OverlapLenRequire, "m", 30, "PE reads重叠区最小长度")

	flag.StringVar(&config.LogLevel, "log-level", "info", "日志级别: debug,info,warn,error")

	defaultThreads := max(1, runtime.NumCPU()/2)
	flag.IntVar(&config.Threads, "threads", defaultThreads, "线程数，默认CPU核心数的一半")

	flag.BoolVar(&config.ContaminationDetection, "contamination-detection", false, "启用交叉污染检测")

	flag.Parse()

	// 必需参数校验
	if config.ExcelFile == "" {
		flag.Usage()
		log.Fatalf("-i 参数必需！: [%+v]	", config)
	}
	return config
}
