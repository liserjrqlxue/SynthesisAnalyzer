package report

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ParseBatchInfo 从BOM文件路径解析Batch信息
// 示例文件路径: test/260305B-引物订购单_BOM.xlsx
// 解析结果: BatchID=260305B, 合成日期=2026.03.05, 仪器号=B
func ParseBatchInfo(bomFile string) (batchID string, synthesisDate string, instrumentID string, err error) {
	// 提取文件名
	fileName := filepath.Base(bomFile)

	// 移除文件扩展名
	fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// 提取BatchID部分（文件名的第一部分，以-分隔）
	parts := strings.Split(fileName, "-")
	if len(parts) == 0 {
		return "", "", "", fmt.Errorf("无效的文件名格式")
	}

	batchID = parts[0][:7]

	// 验证BatchID格式 (YYMMDD+仪器号)
	match, err := regexp.MatchString(`^\d{6}[A-Z]$`, batchID)
	if err != nil {
		return "", "", "", err
	}
	if !match {
		return "", "", "", fmt.Errorf("BatchID格式无效，应为YYMMDD+仪器号")
	}

	// 提取日期部分 (前6位数字)
	dateStr := batchID[:6]

	// 解析日期
	t, err := time.Parse("060102", dateStr)
	if err != nil {
		return "", "", "", fmt.Errorf("日期格式无效: %v", err)
	}

	// 格式化合成日期为 YYYY.MM.DD
	synthesisDate = t.Format("2006.01.02")

	// 提取仪器号 (BatchID的剩余部分)
	instrumentID = batchID[6:]

	return batchID, synthesisDate, instrumentID, nil
}
