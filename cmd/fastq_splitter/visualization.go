package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// 生成可视化数据（用于R或Python绘图）
func (a *AlignmentAnalyzer) generateVisualizationData(reportDir string) error {
	fmt.Println("生成可视化数据...")

	visDir := filepath.Join(reportDir, "visualization")
	if err := os.MkdirAll(visDir, 0755); err != nil {
		return err
	}

	// 1. 生成错误率分布数据
	if err := a.generateErrorRateData(visDir); err != nil {
		return err
	}

	// 2. 生成覆盖度分布数据
	if err := a.generateCoverageData(visDir); err != nil {
		return err
	}

	// 3. 生成合成成功率热图数据
	if err := a.generateSuccessHeatmapData(visDir); err != nil {
		return err
	}

	// 4. 生成R脚本用于绘图
	if err := a.generateRScripts(visDir); err != nil {
		return err
	}

	return nil
}

// 生成R绘图脚本
func (a *AlignmentAnalyzer) generateRScripts(visDir string) error {
	rScript := `# 合成成功率分析R脚本
# 生成时间: %s

# 设置工作目录
setwd("%s")

# 加载必要的库
library(ggplot2)
library(reshape2)
library(plotly)
library(gridExtra)

# 读取数据
summary_data <- read.csv("../alignment_summary.csv", stringsAsFactors = FALSE)

# 1. 整体统计图
p1 <- ggplot(summary_data, aes(x = Sample, y = Synthesis_Success)) +
  geom_bar(stat = "identity", fill = "steelblue") +
  labs(title = "各样品合成成功率", x = "样品", y = "合成成功率%%") +
  theme_minimal() +
  theme(axis.text.x = element_text(angle = 45, hjust = 1))

p2 <- ggplot(summary_data, aes(x = Sample, y = Average_Coverage)) +
  geom_bar(stat = "identity", fill = "darkgreen") +
  labs(title = "各样品平均覆盖度", x = "样品", y = "平均覆盖度") +
  theme_minimal() +
  theme(axis.text.x = element_text(angle = 45, hjust = 1))

# 保存图形
png("summary_plots.png", width = 1600, height = 800)
grid.arrange(p1, p2, ncol = 2)
dev.off()

# 2. 读取每个样品的位置数据并生成热图，同时合并所有position_stats.csv
sample_names <- unique(summary_data$Sample)

# 创建一个空的错误率矩阵
max_length <- max(summary_data$Reference_Length)
error_matrix <- matrix(NA, nrow = max_length, ncol = length(sample_names))
colnames(error_matrix) <- sample_names

# 创建一个空列表来存储所有样品的数据
all_pos_data <- list()

# 填充矩阵并合并数据
for (i in 1:length(sample_names)) {
  sample_name <- sample_names[i]
  pos_file <- sprintf("../%%s/position_stats.csv", sample_name)
  if (file.exists(pos_file)) {
    pos_data <- read.csv(pos_file)
    error_matrix[1:nrow(pos_data), i] <- pos_data$Error_Rate
    
    # 添加样品名列到数据中
    pos_data$Sample <- sample_name
    # 将数据添加到列表
    all_pos_data[[i]] <- pos_data
  }
}

# 合并所有样品的数据
if (length(all_pos_data) > 0) {
  combined_pos_data <- do.call(rbind, all_pos_data)
  
  # 重新排列列的顺序，使Sample列在前
  combined_pos_data <- combined_pos_data[, c("Sample", setdiff(names(combined_pos_data), "Sample"))]
  
  # 输出合并后的CSV文件
  write.csv(combined_pos_data, file = "combined_position_stats.csv", row.names = FALSE)
  cat(sprintf("已成功合并 %%d 个样品的数据到 combined_position_stats.csv\n", length(all_pos_data)))
} else {
  cat("未找到任何position_stats.csv文件\n")
}

# 生成热图
png("error_rate_heatmap.png", width = 1200, height = 800)
heatmap(error_matrix, 
		Rowv=NA,
        main = "错误率热图",
        xlab = "样品",
        ylab = "位置",
        col = colorRampPalette(c("green", "yellow", "red"))(100))
dev.off()

# 3. 生成合成成功率趋势图
png("synthesis_trend.png", width = 1200, height = 600)
par(mfrow = c(2, 3))
for (i in 1:min(6, length(sample_names))) {
  sample_name <- sample_names[i]
  pos_file <- sprintf("../%%s/position_stats.csv", sample_name)
  if (file.exists(pos_file)) {
    pos_data <- read.csv(pos_file)
    plot(pos_data$Position, pos_data$Synthesis_Success, 
         type = "l", 
         main = sample_name,
         xlab = "位置", 
         ylab = "合成成功率%%",
         ylim = c(0, 100),
         col = "blue")
    abline(h = 95, col = "red", lty = 2)
  }
}
dev.off()

print("分析完成！请查看生成的图形文件。")
`

	scriptContent := fmt.Sprintf(rScript,
		time.Now().Format("2006-01-02 15:04:05"),
		visDir)

	scriptFile := filepath.Join(visDir, "synthesis_analysis.R")
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0644); err != nil {
		return err
	}

	fmt.Printf("R脚本已生成: %s\n", scriptFile)
	return nil
}

func (a *AlignmentAnalyzer) generateErrorRateData(visDir string) error {
	// 生成错误率数据
	errorDataFile := filepath.Join(visDir, "error_rate_data.csv")
	f, err := os.Create(errorDataFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{"Sample"}
	for i := 1; i <= 200; i++ { // 假设最大长度200，实际应该动态确定
		header = append(header, fmt.Sprintf("Pos_%d", i))
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		record := []string{sample.Name}
		for _, pos := range sample.AlignmentResult.PositionStats {
			record = append(record, fmt.Sprintf("%.6f", pos.ErrorRate))
		}

		// 填充剩余位置
		for i := len(record); i < len(header); i++ {
			record = append(record, "NA")
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func (a *AlignmentAnalyzer) generateCoverageData(visDir string) error {
	// 生成覆盖度数据
	coverageDataFile := filepath.Join(visDir, "coverage_data.csv")
	f, err := os.Create(coverageDataFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{"Sample"}
	for i := 1; i <= 200; i++ {
		header = append(header, fmt.Sprintf("Cov_%d", i))
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		record := []string{sample.Name}
		for _, pos := range sample.AlignmentResult.PositionStats {
			record = append(record, fmt.Sprintf("%.4f", pos.Coverage))
		}

		// 填充剩余位置
		for i := len(record); i < len(header); i++ {
			record = append(record, "0")
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func (a *AlignmentAnalyzer) generateSuccessHeatmapData(visDir string) error {
	// 生成合成成功率热图数据
	heatmapDataFile := filepath.Join(visDir, "success_heatmap_data.csv")
	f, err := os.Create(heatmapDataFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 写入表头
	header := []string{"Sample"}
	for i := 1; i <= 200; i++ {
		header = append(header, fmt.Sprintf("Pos_%d", i))
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		record := []string{sample.Name}
		for _, pos := range sample.AlignmentResult.PositionStats {
			synthesisSuccess := 100.0
			total := pos.MatchCount + pos.MismatchCount + pos.DeletionCount + pos.InsertionCount
			if total > 0 {
				synthesisSuccess = float64(pos.MatchCount) / float64(total) * 100
			}
			record = append(record, fmt.Sprintf("%.2f", synthesisSuccess))
		}

		// 填充剩余位置
		for i := len(record); i < len(header); i++ {
			record = append(record, "100")
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func (a *AlignmentAnalyzer) generateQCReport() error {
	// 生成质量控制报告
	qcFile := filepath.Join(a.outputDir, "reports", "quality_control_report.txt")
	f, err := os.Create(qcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	qcReport := `质量控制报告
============

评估标准:
1. 合成成功率 > 95%: 优秀
2. 合成成功率 90-95%: 良好
3. 合成成功率 80-90%: 一般
4. 合成成功率 < 80%: 需要优化

样品质量评估:
`
	writer.WriteString(qcReport)

	excellentCount := 0
	goodCount := 0
	fairCount := 0
	poorCount := 0

	for _, sample := range a.samples {
		if sample.AlignmentResult == nil {
			continue
		}

		successRate := sample.AlignmentResult.Summary.SynthesisSuccess
		quality := ""

		if successRate > 95 {
			quality = "优秀"
			excellentCount++
		} else if successRate > 90 {
			quality = "良好"
			goodCount++
		} else if successRate > 80 {
			quality = "一般"
			fairCount++
		} else {
			quality = "需要优化"
			poorCount++
		}

		line := fmt.Sprintf("- %s: %.2f%% (%s)\n", sample.Name, successRate, quality)
		writer.WriteString(line)
	}

	summary := fmt.Sprintf(`
汇总:
- 优秀: %d 个样品
- 良好: %d 个样品
- 一般: %d 个样品
- 需要优化: %d 个样品

建议:
1. 对于成功率<80%的样品，建议优化合成条件
2. 检查高错误率位置的序列特征
3. 考虑使用更长的引物或调整合成参数
`, excellentCount, goodCount, fairCount, poorCount)

	writer.WriteString(summary)

	fmt.Printf("质量控制报告已生成: %s\n", qcFile)
	return nil
}
