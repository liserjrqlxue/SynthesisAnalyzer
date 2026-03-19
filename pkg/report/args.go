package report

import (
	"flag"
	"log"
)

// Config 存储命令行参数
type Config struct {
	InputFile        string
	OutputFile       string
	MutationStatsDir string
	BomFile          string
	EmbedImage       bool
	UseGoEcharts     bool
	ConfigFile       string
	Template         string
	LogLevel         string
}

// ParseArgs 解析命令行参数
func ParseArgs() *Config {
	inputFile := flag.String("i", "", "输入JSON文件路径（可选）")
	outputFile := flag.String("o", "", "输出报告文件路径（默认输出到stdout）")
	mutationStatsDir := flag.String("m", "", "mutation_stats目录路径（可选）")
	bomFile := flag.String("b", "", "BOM.xlsx文件路径（可选）")
	embedImage := flag.Bool("embed-image", false, "是否将图表以Base64编码嵌入HTML（默认false，使用外部图片文件）")
	useGoEcharts := flag.Bool("use-go-echarts", false, "是否使用go-echarts生成图表（默认false，使用内置SVG生成器）")
	configFile := flag.String("c", "", "配置文件路径（可选）")
	template := flag.String("template", "default", "报告模板名称（默认：default）")
	logLevel := flag.String("log-level", "info", "日志级别（可选：debug, info, warn, error）")
	flag.Parse()

	// 设置日志级别
	setLogLevel(*logLevel)

	return &Config{
		InputFile:        *inputFile,
		OutputFile:       *outputFile,
		MutationStatsDir: *mutationStatsDir,
		BomFile:          *bomFile,
		EmbedImage:       *embedImage,
		UseGoEcharts:     *useGoEcharts,
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
