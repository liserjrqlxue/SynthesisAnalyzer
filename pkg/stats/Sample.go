package stats

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 样本结构体
type Sample struct {
	Name       string // 样本名称
	RefSeqFull string // 全长参考序列
	BamFile    string // BAM文件路径

	RefLength int // 参考序列长度
	HeadCut   int // 头切除长度
	TailCut   int // 尾切除长度
}

func (sample *Sample) UpdateFullSeqs() (err error) {
	// 构造可能的参考文件路径
	refCandidates := []string{
		filepath.Join(filepath.Dir(sample.BamFile), sample.Name+".ref.fa"),
		filepath.Join(filepath.Dir(sample.BamFile), "ref.fa"),
	}
	for _, refPath := range refCandidates {
		if _, err = os.Stat(refPath); err == nil {
			sample.RefSeqFull, err = readRefFasta(refPath)
			if err != nil {
				slog.Error("读取参考文件失败", "样品", sample.Name, "参考文件", refPath, "错误", err)
				return err
			}
			break
		}
	}
	if sample.RefSeqFull == "" {
		slog.Error("未找到参考序列文件", "样品", sample.Name)
		err = fmt.Errorf("未找到参考序列文件 %s:[%v]", sample.Name, err)
		return err
	}
	return
}

func (sample *Sample) NewSampleStats() *SampleStats {
	sampleStats := NewSampleStats()

	sampleStats.Sample = sample
	sample.RefLength = len(sample.RefSeqFull)

	// 预计算参考序列信息
	if sample.RefSeqFull != "" {
		if sample.HeadCut+sample.TailCut < sample.RefLength {
			trimmedSeq := sample.RefSeqFull[sample.HeadCut : sample.RefLength-sample.TailCut]
			acgtCounts := make(map[byte]int, 4)
			for i := range trimmedSeq {
				b := trimmedSeq[i]
				if b == 'A' || b == 'C' || b == 'G' || b == 'T' {
					acgtCounts[b]++
				}
			}
			sampleStats.RefACGTCounts = acgtCounts
			sampleStats.RefLengthAfterTrim = len(trimmedSeq)
		} else {
			fmt.Printf("  警告: 样本 %s 的切除长度超过序列全长\n", sample.Name)
		}

		// 基于参考序列长度预定义PositionStats
		sampleStats.PositionStats = make(map[int]*PositionDetail, sample.RefLength)
		for pos := 1; pos <= sample.RefLength; pos++ {
			sampleStats.PositionStats[pos] = &PositionDetail{}
		}
	}

	return sampleStats
}

// 样本信息结构体
type BatchInfo struct {
	InputExcel string // 输入Excel文件路径
	InputSheet string // 输入Sheet名称
	InputDir   string // 输入目录路径
	OutputDir  string // 输出目录路径

	HeadCuts         int // 头切除长度
	TailCuts         int // 尾切除长度
	NMerSize         int // n-mer大小
	MaxSubstitutions int // 最大错配次数
	MaxThreads       int // 最大线程数

	Order   []string           // 样本顺序列表
	Samples map[string]*Sample // 样本名->样本信息
}

func NewSampleInfo() *BatchInfo {
	return &BatchInfo{
		Order:   []string{},
		Samples: make(map[string]*Sample),
	}
}

// readExcelSampleOrder 使用 excelize 读取 Excel 文件，基于表头确定列
// 返回: 样品信息结构体, 错误
func (s *BatchInfo) ReadExcel() error {
	if s.InputExcel == "" {
		slog.Warn("未指定Excel文件路径")
		return nil
	}

	slog.Info("读取Excel文件", "filePath", s.InputExcel)
	f, err := excelize.OpenFile(s.InputExcel)
	if err != nil {
		return fmt.Errorf("打开Excel文件失败: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(s.InputSheet)
	if err != nil {
		return fmt.Errorf("读取Sheet[%s]失败: %v", s.InputSheet, err)
	}
	if len(rows) < 2 {
		return fmt.Errorf("Excel文件[%s]中至少需要表头和一行数据", s.InputSheet)
	}

	// 解析表头，找到各列索引
	header := rows[0]
	nameCol := -1
	targetCol := -1
	synthCol := -1
	postCol := -1

	for i, colName := range header {
		trimmed := strings.TrimSpace(colName)
		switch trimmed {
		case "样品名称":
			nameCol = i
		case "靶标序列":
			targetCol = i
		case "合成序列":
			synthCol = i
		case "后靶标":
			postCol = i
		}
	}

	// 检查必需列
	if nameCol == -1 {
		return fmt.Errorf("未找到'样品名称'列")
	}
	if targetCol == -1 {
		return fmt.Errorf("未找到'靶标序列'列")
	}
	if synthCol == -1 {
		return fmt.Errorf("未找到'合成序列'列")
	}
	if postCol == -1 {
		return fmt.Errorf("未找到'后靶标'列")
	}

	// 遍历数据行（从第二行开始）
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		// 确保行长度足够
		if len(row) <= nameCol || len(row) <= targetCol || len(row) <= synthCol || len(row) <= postCol {
			fmt.Printf("警告: 第 %d 行缺少必要的列数据，跳过\n", i+1)
			continue
		}
		sampleName := strings.TrimSpace(row[nameCol])
		targetSeq := strings.TrimSpace(row[targetCol])
		synthSeq := strings.TrimSpace(row[synthCol])
		postSeq := strings.TrimSpace(row[postCol])

		if sampleName == "" {
			fmt.Printf("警告: 第 %d 行样品名称为空，跳过\n", i+1)
			continue
		}

		fullSeq := strings.ToUpper(targetSeq + synthSeq + postSeq)
		sample := &Sample{
			Name:       sampleName,
			RefSeqFull: fullSeq,
			RefLength:  len(fullSeq),
			HeadCut:    len(targetSeq), // 头切除长度 = 靶标序列长度
			TailCut:    len(postSeq),   // 尾切除长度 = 后靶标长度
		}
		s.Samples[sampleName] = sample
		s.Order = append(s.Order, sampleName)
	}

	if len(s.Order) == 0 {
		return fmt.Errorf("没有有效的数据行")
	}

	return nil
}

// findBAMFiles 查找所有BAM文件，并尝试补充参考序列
func (s *BatchInfo) FindBAMFiles() error {
	if len(s.Order) > 0 {
		return s.findBAMFilesFromExcel()
	} else {
		return s.findBAMFilesFromWalk()
	}
}

// findBAMFilesFromExcel 从Excel文件中获取样本顺序并查找对应BAM文件
func (s *BatchInfo) findBAMFilesFromExcel() (err error) {
	// 使用Excel中的样本顺序
	for _, sampleName := range s.Order {
		sortBam := filepath.Join(s.InputDir, "samples", sampleName, sampleName+".sorted.bam")
		filterBam := filepath.Join(s.InputDir, sampleName, sampleName+".filter.bam")
		var bamPath string
		if _, err = os.Stat(sortBam); err == nil {
			bamPath = sortBam
		} else if _, err = os.Stat(filterBam); err == nil {
			bamPath = filterBam
		} else {
			slog.Error("找不到BAM文件", "样品", sampleName, "sortBam", sortBam, "filterBam", filterBam)
			return
		}
		// 更新样本的BAM文件路径
		sample, ok := s.Samples[sampleName]
		if !ok {
			// Order 与 Samples 冲突，理论应该不会发生
			err = fmt.Errorf("样本 %s 未在Excel中定义", sampleName)
			slog.Error("样本未在Excel中定义", "样品", sampleName)
			return
		}
		sample.BamFile = bamPath

		// 若样本中尚无参考序列，尝试查找 .ref.fa 文件
		if sample.RefSeqFull == "" {
			err = sample.UpdateFullSeqs()
			if err != nil {
				return err
			}
		}
	}
	return
}

// findBAMFilesFromWalk 递归遍历目录查找BAM文件
func (s *BatchInfo) findBAMFilesFromWalk() (err error) {
	// 原来的方法：递归查找所有BAM文件
	err = filepath.Walk(s.InputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (strings.HasSuffix(path, ".sorted.bam") || strings.HasSuffix(path, ".filter.bam")) {
			// ===== 新增：尝试补充参考序列 =====
			sampleName := filepath.Base(filepath.Dir(path))

			// 检查样本是否已存在，不存在则创建
			sample, ok := s.Samples[sampleName]
			if !ok {
				sample = &Sample{
					Name:    sampleName,
					HeadCut: s.HeadCuts,
					TailCut: s.TailCuts,
				}
				s.Samples[sampleName] = sample
				s.Order = append(s.Order, sampleName)
			}

			// 更新BAM文件路径
			sample.BamFile = path

			// 若样本中尚无参考序列，尝试查找 .ref.fa 文件
			if sample.RefSeqFull == "" {
				err = sample.UpdateFullSeqs()
				if err != nil {
					return err
				}
			}
		}

		return nil
	})

	return
}
