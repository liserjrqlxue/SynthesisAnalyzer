package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"SynthesisAnalyzer/pkg/report"
)

// ------------------------------------------------------------
// 主函数：读取JSON并生成报告
// ------------------------------------------------------------
func main() {
	// 获取可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("警告：获取可执行文件路径失败: %v", err)
	}
	// 获取可执行文件所在目录
	execDir := filepath.Dir(execPath)

	// 解析命令行参数
	config := report.ParseArgs(execDir)

	// 加载并处理数据
	processor := report.NewProcessor(config)
	reportData, err := processor.LoadData()
	if err != nil {
		log.Fatalf("加载数据失败: %v", err)
	}

	// 生成报告
	generator := report.NewGenerator(config)

	// 输出
	if config.Prefix != "" {
		// 生成HTML报告
		htmlReport := generator.GenerateHTML(reportData)
		htmlFile := config.Prefix + ".html"
		if err := os.WriteFile(htmlFile, []byte(htmlReport), 0644); err != nil {
			log.Fatalf("写入HTML文件失败: %v", err)
		}
		log.Printf("HTML报告已生成: %s", htmlFile)

		// 生成TXT报告
		txtReport := generator.GenerateTXT(reportData)
		txtFile := config.Prefix + ".txt"
		if err := os.WriteFile(txtFile, []byte(txtReport), 0644); err != nil {
			log.Fatalf("写入TXT文件失败: %v", err)
		}
		log.Printf("TXT报告已生成: %s", txtFile)
	} else {
		// 默认输出到stdout
		htmlReport := generator.GenerateHTML(reportData)
		fmt.Print(htmlReport)
	}
}
