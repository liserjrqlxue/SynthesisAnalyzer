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
	Name       string // 样本名称
	RefSeqFull string // 全长参考序列
	BamFile    string // BAM文件路径
	RefLength  int    // 参考序列长度
	HeadCut    int    // 头切除长度
	TailCut    int    // 尾切除长度
}
```

### BatchInfo 结构体

```go
type BatchInfo struct {
	InputExcel       string // 输入Excel文件路径
	InputSheet       string // 输入Sheet名称
	InputDir         string // 输入目录路径
	OutputDir        string // 输出目录路径
	SampleNameSuffix string // 样品名称后缀列

	HeadCuts         int // 头切除长度
	TailCuts         int // 尾切除长度
	NMerSize         int // n-mer大小
	MaxSubstitutions int // 最大错配次数
	MaxThreads       int // 最大线程数

	Order   []string           // 样本顺序列表
	Samples map[string]*Sample // 样本名->样本信息
}
```

### SampleStats 结构体

存储单个样本的统计信息，包括各种突变类型的计数、分布等。

| 类别 | 字段 | 类型 | 说明 |
|------|----------|------|------|
| 基本统计 | ReadCounts, AlignedReads, ReadsWithMutations | int | 总reads数、比对reads数、含突变的reads数 |
| Reads维度 | InsertReads, DeleteReads, SubstitutionReads | int | 包含插入、缺失、替换的reads数 |
| 事件维度 | InsertEventCount, DeleteEventCount, SubstitutionEventCount | int | 插入、缺失、替换事件个数 |
| 碱基维度 | InsertBaseTotal, DeleteBaseTotal, SubstitutionBaseTotal | int | 插入、缺失、替换碱基总数 |
| 缺失细分类 | DeleteSubtypeReads, DeleteSubtypeEvents, DeleteSubtypeBases | map[DeletionSubtype]int | 缺失细分类统计 |
| 插入细分类 | InsertSubtypeReads, InsertSubtypeEvents, InsertSubtypeBases | map[InsertionSubtype]int | 插入细分类统计 |
| 替换细分类 | SubstitutionSubtypeReads, SubstitutionSubtypeEvents, SubstitutionSubtypeBases | map[SubstitutionSubtype]int | 替换细分类统计 |
| 组合统计 | SubtypeCombinationCounts | map[string]int | 细分类组合统计 |
| Del1统计 | Del1BaseCounts | map[byte]int | Del1缺失碱基分布 |
| Del3统计 | Del3PrevBaseCounts, Del3FirstBaseCounts, Del3PrevFirstCombCounts | map[byte]int / map[string]int | Del3缺失相关统计 |
| 位置统计 | PositionStats | map[int]*PositionDetail | 位置级详细统计 |
| 参考统计 | RefACGTCounts, RefLengthAfterTrim | map[byte]int / int | 参考序列统计 |
| 替换分布 | SubstitutionCountDist | map[int]int | 替换个数分布 |
| 质量统计 | GoodAlignedReads | int | 良好比对的reads数 |

### MutationStats 结构体

汇总所有样本的统计结果，生成总体统计报告。

| 类别 | 字段 | 类型 | 说明 |
|------|----------|------|------|
| 基本统计 | TotalReadCount, TotalAlignedReads, TotalReadsWithMuts | int | 总reads数、比对reads数、含突变的reads数 |
| Reads维度 | TotalInsertReads, TotalDeleteReads, TotalSubstitutionReads | int | 包含插入、缺失、替换的reads数 |
| 事件维度 | TotalInsertEventCount, TotalDeleteEventCount, TotalSubstitutionEventCount | int | 插入、缺失、替换事件个数 |
| 碱基维度 | TotalInsertBaseTotal, TotalDeleteBaseTotal, TotalSubstitutionBaseTotal | int | 插入、缺失、替换碱基总数 |
| 缺失细分类 | TotalDeleteSubtypeReads, TotalDeleteSubtypeEvents, TotalDeleteSubtypeBases | map[DeletionSubtype]int | 缺失细分类统计 |
| 插入细分类 | TotalInsertSubtypeReads, TotalInsertSubtypeEvents, TotalInsertSubtypeBases | map[InsertionSubtype]int | 插入细分类统计 |
| 替换细分类 | TotalSubstitutionSubtypeReads, TotalSubstitutionSubtypeEvents, TotalSubstitutionSubtypeBases | map[SubstitutionSubtype]int | 替换细分类统计 |
| 组合统计 | TotalSubtypeCombinationCounts | map[string]int | 细分类组合统计 |
| Del3统计 | TotalDel3PrevBaseCounts, TotalDel3FirstBaseCounts, TotalDel3PrevFirstCombCounts | map[byte]int / map[string]int | Del3缺失相关统计 |
| 替换分布 | TotalSubstitutionCountDist | map[int]int | 替换个数分布 |
| 参考统计 | TotalRefACGTCounts, TotalRefLengthAfterTrim | map[byte]int / int | 参考序列统计 |
| 质量统计 | TotalGoodAlignedReads, TotalRefLengthGoodAligned, TotalAlignedBases | int | 良好比对的reads数等质量统计 |

### PositionDetail 结构体

| 字段 | 类型 | 说明 |
|------|------|------|
| Base | byte | 参考碱基 (ACGTN) |
| Depth | int | 深度 |
| MatchPure | int | 纯匹配数 |
| MatchWithIns | int | 带插入的匹配数 |
| MismatchPure | int | 纯错配数 |
| MismatchWithIns | int | 带插入的错配数 |
| Insertion | int | 插入数 |
| Deletion | int | 缺失数 |
| Del1 | int | 长度1缺失数 |
| PerfectReadsCount | int | 完美reads数 |
| PerfectUptoPosCount | int | 到当前位置完美的reads数 |
| NCorrect | int | N-mer正确数 |
| N1Correct | int | N-mer首碱基正确数 |

## 4. 细分类类型定义

| 类型 | 子类型 | 说明 |
|------|--------|------|
| **DeletionSubtype** | Del1 | 长度=1的缺失 |
| | Del2 | 长度=2的缺失 |
| | Del3 | 长度>2的缺失 |
| **InsertionSubtype** | Dup1 | 长度1，与-1或+1碱基一致（重复） |
| | Dup2 | 长度2，插入序列一致且与-1或+1碱基一致 |
| | DupDup | 长度2，插入序列不一致，第一位与-1一致，第二位与+1一致 |
| | Ins1 | 长度1，非Dup1 |
| | Ins2 | 长度2，非Dup2/DupDup |
| | Ins3 | 长度>2 |
| **SubstitutionSubtype** | DupDel | 替换碱基与-1位相同 |
| | DelDup | 替换碱基与+1位相同 |
| | MismatchA_G | A>G替换 |
| | MismatchC_T | C>T替换 |
| | OtherMismatch | 其他错配 |

## 5. 核心功能

### 样本信息管理

- `ReadExcel`：从Excel文件读取样本信息，支持样品名称后缀拼接和重复性检测
- `FindBAMFiles`：查找BAM文件并补充参考序列
- `findBAMFilesFromExcel`：从Excel文件中获取样本顺序并查找对应BAM文件
- `findBAMFilesFromWalk`：递归遍历目录查找BAM文件

### 突变分析

- `ProcessBAMFile`：处理单个BAM文件，分析突变情况
- `analyzeReadDetailedInfo`：分析read的详细类型和突变信息
- `parseMDToMap`：使用MD标签解析突变

### 统计报告

- `MainWrite`：生成各种统计文件
- `writeTotalMutationStats`：生成总突变统计文件
- `writeReadTypeBySample`：生成样本read类型统计文件
- `writeReadSubtypeStats`：生成细分类统计文件
- `MainPrint`：打印统计结果到控制台

## 6. 使用示例

### 基本使用

```go
// 初始化样本信息
sampleInfo := stats.NewSampleInfo()
sampleInfo.InputExcel = "samples.xlsx"
sampleInfo.InputDir = "input_dir"
sampleInfo.HeadCuts = 27
sampleInfo.TailCuts = 20

// 读取Excel文件
if err := sampleInfo.ReadExcel(); err != nil {
	log.Fatalf("读取Excel文件失败: %v", err)
}

// 查找BAM文件
if err := sampleInfo.FindBAMFiles(); err != nil {
	log.Fatalf("查找BAM文件失败: %v", err)
}

// 处理BAM文件
mutationStats := stats.NewMutationStats()
mutationStats.BatchInfo = sampleInfo
if err := mutationStats.ProcessBAMFiles(); err != nil {
	log.Fatalf("处理BAM文件失败: %v", err)
}

// 生成统计文件
mutationStats.MainWrite()
```

## 7. 性能优化

- **并发处理**：使用goroutine并发处理多个BAM文件，通过信号量控制并发数
- **内存优化**：合理使用内存，避免内存泄漏，预分配缓冲区减少GC
- **I/O优化**：减少I/O操作，提高处理速度
- **字符串处理优化**：改进MD字符串解析，使用直接切片替代strings.TrimPrefix

## 8. 故障排除

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

4. **样品名称重复**
   - 错误信息：`第 X 行样品名称重复: XXX`
   - 解决方法：确保Excel文件中的样品名称唯一，或使用-suffix-col参数添加后缀

## 9. 版本历史

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

### v1.4.0
- 新增SampleNameSuffix参数，支持样品名称后缀拼接
- 增加样品名称重复性检测
- 优化并发处理逻辑
- 增加更多统计维度和字段

---

**版本**：1.4.0
**最后更新**：2026-03-27

