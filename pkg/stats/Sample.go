package stats

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"SynthesisAnalyzer/pkg/cfg"
)

// 样本信息结构体
type BatchInfo struct {
	Config *cfg.Config // 配置信息

	SampleList []string               // 样本顺序列表
	Samples    []*cfg.Sample          // 样本列表
	SampleMap  map[string]*cfg.Sample // 样本名->样本信息
}

func NewBatchInfo() *BatchInfo {
	return &BatchInfo{
		SampleList: []string{},
		SampleMap:  make(map[string]*cfg.Sample),
	}
}

// ReadExcel 使用 excelize 读取 Excel 文件，基于表头确定列
// 返回: 样品信息结构体, 错误
func (s *BatchInfo) ReadExcel() error {
	samples, err := s.Config.LoadInputExcel()
	if err != nil {
		return fmt.Errorf("读取Excel文件失败: %v", err)
	}

	s.Samples = samples
	for _, sample := range samples {
		s.SampleList = append(s.SampleList, sample.Name)
		s.SampleMap[sample.Name] = sample
	}

	if len(s.SampleList) == 0 {
		return fmt.Errorf("没有有效的数据行")
	}

	return nil
}

// findBAMFiles 查找所有BAM文件，并尝试补充参考序列
func (s *BatchInfo) FindBAMFiles() error {
	if len(s.SampleList) > 0 {
		return s.findBAMFilesFromExcel()
	} else {
		return s.findBAMFilesFromWalk()
	}
}

// findBAMFilesFromExcel 从Excel文件中获取样本顺序并查找对应BAM文件
func (s *BatchInfo) findBAMFilesFromExcel() (err error) {
	// 使用Excel中的样本顺序
	for _, sampleName := range s.SampleList {
		sortBam := filepath.Join(s.Config.InputDir, "samples", sampleName, sampleName+".sorted.bam")
		filterBam := filepath.Join(s.Config.InputDir, sampleName, sampleName+".filter.bam")
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
		sample, ok := s.SampleMap[sampleName]
		if !ok {
			// Order 与 Samples 冲突，理论应该不会发生
			err = fmt.Errorf("样本 %s 未在Excel中定义", sampleName)
			slog.Error("样本未在Excel中定义", "样品", sampleName)
			return
		}
		sample.BamFile = bamPath

		// 若样本中尚无参考序列，尝试查找 .ref.fa 文件
		if sample.FullReference == "" {
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
	err = filepath.Walk(s.Config.InputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (strings.HasSuffix(path, ".sorted.bam") || strings.HasSuffix(path, ".filter.bam")) {
			// ===== 新增：尝试补充参考序列 =====
			sampleName := filepath.Base(filepath.Dir(path))

			// 检查样本是否已存在，不存在则创建
			sample, ok := s.SampleMap[sampleName]
			if !ok {
				sample = &cfg.Sample{
					Name:    sampleName,
					HeadCut: s.Config.HeadCut,
					TailCut: s.Config.TailCut,
				}
				s.SampleMap[sampleName] = sample
				s.SampleList = append(s.SampleList, sampleName)
			}

			// 更新BAM文件路径
			sample.BamFile = path

			// 若样本中尚无参考序列，尝试查找 .ref.fa 文件
			if sample.FullReference == "" {
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
