package main

import (
	"fmt"
	"log"
	"os"

	"SynthesisAnalyzer/pkg/report"
)

// ------------------------------------------------------------
// 主函数：读取JSON并生成报告
// ------------------------------------------------------------
func main() {
	// 解析命令行参数
	config := report.ParseArgs()

	// 加载并处理数据
	processor := report.NewProcessor(config)
	reportData, err := processor.LoadData()
	if err != nil {
		log.Fatalf("加载数据失败: %v", err)
	}

	// 生成报告
	generator := report.NewGenerator(config)
	htmlReport := generator.GenerateHTML(reportData)

	// 输出
	if config.OutputFile != "" {
		if err := os.WriteFile(config.OutputFile, []byte(htmlReport), 0644); err != nil {
			log.Fatalf("写入文件失败: %v", err)
		}
		log.Printf("报告已生成: %s", config.OutputFile)
	} else {
		fmt.Print(htmlReport)
	}
}
