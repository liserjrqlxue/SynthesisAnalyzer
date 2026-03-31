package report

import (
	"flag"
	"log"
	"path/filepath"
)

// Config 存储命令行参数
type Config struct {
	InputFile        string
	Prefix           string // 输出文件前缀
	MutationStatsDir string
	BomFile          string
	SuffixCol        string // 样品名称后缀列
	BatchID          string // 手动指定BatchID
	EmbedImage       bool
	UseGoEcharts     bool
	UseLocalEcharts  bool   // 是否使用本地echarts资源
	EchartsPath      string // echarts资源路径
	ConfigFile       string
	Template         string
	LogLevel         string
}

// ParseArgs 解析命令行参数
func ParseArgs(execDir string) *Config {
	inputFile := flag.String("i", "", "输入JSON文件路径（可选）")
	prefix := flag.String("p", "", "输出文件前缀（默认输出到stdout）")
	mutationStatsDir := flag.String("m", "", "mutation_stats目录路径（必填）")
	bomFile := flag.String("b", "", "BOM.xlsx文件路径（必填）")
	suffixCol := flag.String("suffix-col", "", "可选参数：样品名称后缀列，若指定则将该列值拼接到样品名称后")
	batchID := flag.String("batch", "", "手动指定BatchID（可选）")
	embedImage := flag.Bool("embed-image", false, "是否将图表以Base64编码嵌入HTML（默认false，使用外部图片文件）")
	useGoEcharts := flag.Bool("use-go-echarts", false, "是否使用go-echarts生成图表（默认false，使用内置SVG生成器）")
	// 计算echarts.min.js的默认路径
	defaultEchartsPath := filepath.Join(execDir, "static", "js", "echarts.min.js")
	useLocalEcharts := flag.Bool("use-local-echarts", true, "是否使用本地echarts资源（默认true）")
	echartsPath := flag.String("echarts-path", defaultEchartsPath, "echarts资源路径")
	configFile := flag.String("c", "", "配置文件路径（可选）")
	template := flag.String("template", "default", "报告模板名称（默认：default）")
	logLevel := flag.String("log-level", "info", "日志级别（可选：debug, info, warn, error）")
	flag.Parse()

	// 设置日志级别
	setLogLevel(*logLevel)

	// 检查必填参数
	if *bomFile == "" {
		log.Fatal("请使用 -b 指定BOM.xlsx文件")
	}
	if *mutationStatsDir == "" {
		log.Fatal("请使用 -m 指定mutation_stats目录")
	}

	return &Config{
		InputFile:        *inputFile,
		Prefix:           *prefix,
		MutationStatsDir: *mutationStatsDir,
		BomFile:          *bomFile,
		SuffixCol:        *suffixCol,
		BatchID:          *batchID,
		EmbedImage:       *embedImage,
		UseGoEcharts:     *useGoEcharts,
		UseLocalEcharts:  *useLocalEcharts,
		EchartsPath:      *echartsPath,
		ConfigFile:       *configFile,
		Template:         *template,
		LogLevel:         *logLevel,
	}
}

// setLogLevel 设置日志级别
func setLogLevel(level string) {
	switch level {
	case "debug":
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	case "info":
		log.SetFlags(log.LstdFlags)
	case "warn":
		// 这里可以实现只显示警告及以上级别的日志
	case "error":
		// 这里可以实现只显示错误级别的日志
	default:
		log.SetFlags(log.LstdFlags)
	}
}
