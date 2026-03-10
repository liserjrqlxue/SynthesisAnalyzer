package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// 全局变量，用于存储命令行参数
var (
	inputDir    string
	outputDir   string
	excelFile   string
	sampleOrder []string // 样本顺序列表

	headCut  int
	tailCut  int
	nMerSize int

	maxSubstitutions int // 最大替换个数阈值

	// fullLengths map[string]int
	fullSeqs = make(map[string]string)
	headCuts map[string]int
	tailCuts map[string]int
)

func init() {
	// 定义命令行参数
	flag.StringVar(&inputDir, "d", "", "输入目录，包含样本子目录")
	flag.StringVar(&outputDir, "o", "", "输出目录, 默认输入目录/mutation_stats")
	flag.StringVar(&excelFile, "i", "", "可选参数：输入Excel文件，包含样本顺序")
	flag.IntVar(&headCut, "head", 27, "头切除长度")
	flag.IntVar(&tailCut, "tail", 20, "尾切除长度")
	flag.IntVar(&maxSubstitutions, "max-sub", 5, "最大替换个数阈值，用于定义比对良好reads")
	flag.IntVar(&nMerSize, "n", 4, "N-mer 统计的 N 值（默认4，即统计5-mer准确率）")
}

func main() {
	// 解析命令行参数
	flag.Parse()

	// 验证必需参数
	if inputDir == "" {
		fmt.Println("错误: 必需参数缺失")
		fmt.Println("使用方法:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 检查输入目录是否存在
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		fmt.Printf("错误: 输入目录不存在: %s\n", inputDir)
		os.Exit(1)
	}

	// 如果指定了Excel文件，读取样本顺序
	if excelFile != "" {
		fmt.Printf("读取Excel文件: %s\n", excelFile)
		var err error
		sampleOrder, fullSeqs, headCuts, tailCuts, err = readExcelSampleOrder(excelFile)
		if err != nil {
			fmt.Printf("错误: 读取Excel文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("从Excel读取到 %d 个样本\n", len(sampleOrder))
	}

	// 创建输出目录
	if outputDir == "" {
		outputDir = filepath.Join(inputDir, "mutation_stats")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// 查找所有BAM文件
	bamFiles, err := findBAMFiles(inputDir, fullSeqs)
	if err != nil {
		fmt.Printf("查找BAM文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("找到 %d 个BAM文件\n", len(bamFiles))

	// 统计处理
	var stats = processBAMFiles(bamFiles)

	fmt.Println("\n开始生成统计文件...")

	mainWrite(outputDir, stats)

	mainPrint(stats)
}
