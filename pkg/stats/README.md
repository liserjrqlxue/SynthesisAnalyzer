# stats 包使用指南

## 1. 功能介绍

`stats` 包是 bam-mut-analyzer 工具的核心统计功能包，提供了DNA合成过程中突变分析的核心功能。

### 主要功能

- **样本信息管理**：管理样本的基本信息，包括参考序列、BAM文件路径等
- **突变检测**：检测并分类替换、插入、缺失等突变类型
- **细分类分析**：对突变进行细分类，如Del1、Del2、Del3等缺失类型
- **统计报告**：生成多种统计报告，包括突变频谱、长度分布等
- **数据写入**：将统计结果写入CSV文件

## 2. 包结构

### 主要文件

- **Sample.go**：样本信息管理，定义了Sample结构体和相关方法
- **SampleStats.go**：样本统计信息，处理单个样本的突变统计
- **MutationStats.go**：突变统计信息，汇总所有样本的统计结果
- **sam.go**：SAM/BAM文件解析，处理BAM文件的读取和分析
- **writer.go**：输出文件写入逻辑，生成各种统计CSV文件
- **struct.go**：数据结构定义，包含各种统计相关的结构体
- **type.go**：类型定义，定义了突变类型、细分类等
- **util.go**：工具函数，提供辅助功能

## 3. 核心数据结构

### Sample 结构体

```go
type Sample struct {
	Name     string // 样本名称
	FullSeqs string // 全长参考序列
	BamFile  string // BAM文件路径
	HeadCuts int    // 头切除长度
	TailCuts int    // 尾切除长度
}
```

### SampleInfo 结构体

```go
type SampleInfo struct {
	Order   []string       // 样本顺序列表
	Samples map[string]*Sample // 样本名->样本信息
}
```

### SampleStats 结构体

存储单个样本的统计信息，包括各种突变类型的计数、分布等。

### MutationStats 结构体

汇总所有样本的统计结果，生成总体统计报告。

## 4. 核心功能

### 样本信息管理

- `readExcelSampleOrder`：从Excel文件读取样本信息
- `findBAMFiles`：查找BAM文件并补充参考序列

### 突变分析

- `processBAMFile`：处理单个BAM文件，分析突变情况
- `analyzeReadType`：分析read的类型
- `parseMutationsWithMD`：使用MD标签解析突变

### 统计报告

- `mainWrite`：生成各种统计文件
- `writeTotalMutationStats`：生成总突变统计文件
- `writeReadTypeBySample`：生成样本read类型统计文件
- `writeReadSubtypeStats`：生成细分类统计文件

## 5. 使用示例

### 基本使用

```go
// 读取Excel文件获取样本信息
sampleInfo, err := readExcelSampleOrder("samples.xlsx")
if err != nil {
	log.Fatalf("读取Excel文件失败: %v", err)
}

// 查找BAM文件
bamFiles, err := findBAMFiles("input_dir", sampleInfo)
if err != nil {
	log.Fatalf("查找BAM文件失败: %v", err)
}

// 处理BAM文件
stats := processBAMFiles(bamFiles, sampleInfo)

// 生成统计文件
mainWrite("output_dir", stats)
```

## 6. 性能优化

- **并发处理**：使用goroutine并发处理多个BAM文件
- **内存优化**：合理使用内存，避免内存泄漏
- **I/O优化**：减少I/O操作，提高处理速度
- **字符串处理优化**：改进MD字符串解析，使用直接切片替代strings.TrimPrefix

## 7. 故障排除

### 常见错误及解决方法

1. **Excel文件读取失败**
   - 错误信息：`未找到'样品名称'列`
   - 解决方法：确保Excel文件包含所有必需的列

2. **BAM文件分析失败**
   - 错误信息：`assignment to entry in nil map`
   - 解决方法：确保BAM文件格式正确，包含必要的标签

3. **内存不足**
   - 错误信息：`out of memory`
   - 解决方法：减少并发处理的文件数量，或增加系统内存

## 8. 版本历史

### v1.0.0
- 初始版本
- 支持基本的突变分析功能

### v1.1.0
- 增加细分类统计
- 优化性能
- 修复bug

### v1.2.0
- 增加Del3相关统计
- 优化内存使用
- 改进报告格式

### v1.3.0
- 重构为pkg/stats包
- 优化代码结构
- 改进样本信息管理

---

**版本**：1.3.0
**最后更新**：2026-03-26