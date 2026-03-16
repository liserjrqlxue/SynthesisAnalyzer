# report 使用指南

## 1. 功能介绍

report 是一个用于生成分析报告的工具，能够汇总 fastq_splitter 和 bam-mut-analyzer 的分析结果，生成综合报告。

### 主要功能

- **报告生成**：生成综合分析报告
- **结果汇总**：汇总 fastq_splitter 和 bam-mut-analyzer 的结果
- **数据可视化**：生成可视化图表数据
- **自定义报告**：支持自定义报告内容和格式

## 2. 安装方法

### 前提条件
- Go 1.16 或更高版本
- 相关依赖库

### 编译步骤

```bash
# 进入项目目录
cd /path/to/SynthesisAnalyzer

# 编译report
go build -o bin/report cmd/report/main.go cmd/report/struct.go cmd/report/tools.go cmd/report/print.go

# 或直接运行
go run cmd/report/main.go [参数]
```

## 3. 使用方法

### 基本用法

```bash
# 基本用法
report -i <输入JSON文件> -o <输出报告文件> -m <mutation_stats目录> -b <BOM.xlsx文件>

# 示例
report -i test/input.json -o test/output.html -m test/output -b test/BOM.xlsx
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-i` | 输入JSON文件路径 | 必需 |
| `-o` | 输出报告文件路径（默认输出到stdout） | 可选 |
| `-m` | mutation_stats目录路径（可选） | 可选 |
| `-b` | BOM.xlsx文件路径（可选） | 可选 |

## 4. 输入文件格式

### 支持的输入文件

- **JSON文件**：包含基本信息和孔位数据
- **BOM.xlsx文件**：包含孔位和引物名称对应关系（转换为txt文件后使用）
- **bam-mut-analyzer结果**：
  - `mutation_stats/read_type_summary.csv`：用于子类型分类
  - `mutation_stats/read_type_by_sample.csv`：用于收率和错误统计

### BOM.xlsx文件格式

BOM.xlsx文件应包含"引物订购单"工作表，格式如下：

| 序号 | 位置 | 引物名称 | 是否重合 | 预测收率 | 序列 |
|------|------|----------|----------|----------|------|
| 1 | A1 | EGG281A0P1 | 是/否 | 30.5 | ATCG... |
| 2 | A2 | EGG281A0P2 | 否 | 45.2 | GCTA... |
| ... | ... | ... | ... | ... | ... |

其中，"是否重合"、"预测收率"和"序列"为可选列。

## 5. 数据字段来源和计算

### 收率统计

- **收率**：从`read_type_by_sample.csv`的"匹配"和"GoodAlignedReads"列计算，公式为：`收率 = (匹配 / GoodAlignedReads) * 100%`
- **收率统计值**：基于所有样本的收率计算平均值、标准差、中位数、四分位数等

### 错误统计

- **缺失**：从`read_type_by_sample.csv`的"DeleteReads"和"GoodAlignedReads"列计算，公式为：`缺失 = (DeleteReads / GoodAlignedReads) * 100%`
- **突变**：从`read_type_by_sample.csv`的"SubstitutionReads"和"GoodAlignedReads"列计算，公式为：`突变 = (SubstitutionReads / GoodAlignedReads) * 100%`
- **插入**：从`read_type_by_sample.csv`的"InsertReads"和"GoodAlignedReads"列计算，公式为：`插入 = (InsertReads / GoodAlignedReads) * 100%`
- **错误子类型**：从`read_type_summary.csv`的细分类统计部分获取，直接使用子类型代码作为键，在生成报告时通过映射转换为用户友好的错误类型名称

### 参考值处理

- **平均收率参考值**：从输入JSON文件的`error_stats_ref`字段获取，如果存在且大于0，则在报告中显示为"平均收率: X.XX% (参考值Y.YY%)"，否则显示为"平均收率: X.XX%"
- **其他错误类型参考值**：从输入JSON文件的`error_stats_ref`字段获取，在合成错误统计表格中显示

## 6. 数据结构

### 主要数据结构

#### Well（孔位数据）

| 字段 | 类型 | 说明 | 来源 |
|------|------|------|------|
| Row | int | 行号（1-12） | BOM.xlsx |
| ColLetter | string | 列字母（H,G,F,E,D,C,B,A） | BOM.xlsx |
| Name | string | 引物名称 | BOM.xlsx |
| IsOverlap | bool | 是否重合序列 | BOM.xlsx（可选） |
| PredictedYield | float64 | 预测收率 (%) | BOM.xlsx（可选） |
| Yield | float64 | 实际收率 (%) | read_type_by_sample.csv |
| Deletion | float64 | 缺失 (%) | read_type_by_sample.csv |
| Mutation | float64 | 突变 (%) | read_type_by_sample.csv |
| Insertion | float64 | 插入 (%) | read_type_by_sample.csv |
| Sequence | string | 序列 | BOM.xlsx（可选） |
| Position | string | 位置 | BOM.xlsx |
| PositionStats | []PositionStats | 位置统计数据 | {ID}_position_detailed.csv |

#### SummaryStats（概要统计）

| 字段 | 类型 | 说明 | 计算方法 |
|------|------|------|----------|
| AvgYield | float64 | 平均收率 | 所有样本收率的平均值 |
| YieldStddev | float64 | 收率标准差 | 所有样本收率的标准差 |
| YieldMedian | float64 | 收率中位数 | 所有样本收率的中位数 |
| YieldQuartile | float64 | 收率四分位数 | 所有样本收率的四分位数 |
| YieldLt1Count | int | 收率<1%片段个数 | 统计收率<1%的样本数 |
| YieldLt5Count | int | 收率<5%片段个数 | 统计收率<5%的样本数 |
| PredictedDifficultSeq | int | 预测难度序列 | 占位值 |
| OverlapSequences | int | 重合序列 | 统计IsOverlap为true的样本数 |

#### ErrorStat（错误统计）

| 字段 | 类型 | 说明 | 来源 |
|------|------|------|------|
| ErrorType | string | 错误类型 | 固定顺序 |
| Reference | *float64 | 参考值 | JSON文件 |
| Data | *float64 | 实际数据 | read_type_summary.csv |

#### PositionStats（位置统计数据）

| 字段 | 类型 | 说明 | 计算方法 |
|------|------|------|----------|
| Pos | int | 位置 | {ID}_position_detailed.csv |
| AvgYield | float64 | 平均收率 | (match_pure / depth) * 100 |
| AvgDeletion | float64 | 平均缺失 | (deletion / depth) * 100 |
| AvgMutation | float64 | 平均突变 | ((mismatch_pure + mismatch_with_ins) / depth) * 100 |
| AvgInsertion | float64 | 平均插入 | (insertion / depth) * 100 |

## 7. 报告结构和格式

### 报告结构

1. **Summary**
   - 基本信息：合成日期、仪器号、合成孔数、合成长度、合成工艺版本、SEC1工艺版本、测序日期
   - 统计信息：平均收率、收率标准差、收率中位数、收率四分位数、收率<1%片段个数、收率<5%片段个数、预测难度序列、重合序列
   - 合成错误统计：按照固定顺序输出13种错误子类型的统计数据

2. **合成板位分析**
   - 合成排板：显示每个孔位的引物名称
   - 预测合成收率：显示每个孔位的预测收率
   - 收率板位统计：显示每个孔位的实际收率
   - 缺失板位统计：显示每个孔位的缺失率
   - 突变板位统计：显示每个孔位的突变率
   - 插入板位统计：显示每个孔位的插入率

3. **合成轮次分析**
   - 平均合成收率随轮次变化
   - 平均缺失随轮次变化
   - 平均突变随轮次变化
   - 平均插入随轮次变化

4. **附录**
   - 单孔单轮信息——收率
   - 单孔单轮信息——缺失
   - 单孔单轮信息——插入
   - 单孔单轮信息——突变

### 错误类型固定顺序

报告中的错误类型按照以下顺序固定输出，如果没有值则留空：

| 错误类型 | 子类型 |
|---------|--------|
| 缺失1 | DEL1 |
| 连续两个缺失 | DEL2 |
| 连续三个及以上缺失 | DEL3 |
| A-G突变 | Mismatch-AG |
| C-T突变 | Mismatch-CT |
| 其余突变 | OtherMismatch |
| 重复插入1个 | DUP1 |
| 重复插入2个 | DUP2 |
| 插入1（与前后不同） | INS1 |
| 插入2（与前后不同） | INS2 |
| 插入3个及以上（与前后不同） | INS3 |
| 缺失+重复 | DELDUP |
| 重复+缺失 | DUPDEL |

## 8. 输出文件说明

### 主要输出文件

- **HTML报告**：`report.html`，包含完整的分析结果和板位统计

## 9. 核心流程

1. 读取命令行参数
2. 读取输入JSON文件
3. 如果提供了BOM.xlsx文件，读取并更新孔位信息：
   - 解析位置、引物名称、是否重合、预测收率和序列信息
   - 跳过引物名称为"0"的行
   - 计算序列长度的最大值，作为合成长度
   - 更新孔数为实际孔位数量
4. 如果提供了mutation_stats目录，读取并更新数据：
   - 从`read_type_summary.csv`读取子类型统计数据
   - 从`read_type_by_sample.csv`读取收率和错误统计数据
   - 从`total_position_detailed.csv`读取总位置统计数据
   - 从每个样品的`{ID}_position_detailed.csv`读取样品位置统计数据
   - 更新每个孔的收率、缺失、突变和插入数据
   - 计算收率统计值（平均值、标准差、中位数、四分位数等）
5. 生成HTML报告：
   - 基本信息部分
   - 统计信息部分（根据参考值是否存在调整平均收率的显示格式）
   - 合成错误统计部分（按照固定顺序输出错误子类型，使用映射转换为用户友好的错误类型名称）
   - 合成板位分析部分
6. 输出结果

## 10. 示例分析流程

### 步骤1：准备输入数据

1. 运行 fastq_splitter 拆分FASTQ文件
2. 运行 bam-mut-analyzer 分析BAM文件
3. 准备BOM.xlsx文件
4. 准备输入JSON文件

### 步骤2：运行报告生成

```bash
# 运行report
report -i test/input.json -o test/output.html -m test/output -b test/BOM.xlsx

# 查看报告
open test/output.html
```

## 11. 代码结构

### 主要文件

- **main.go**：主入口，处理命令行参数，读取输入数据，更新收率和错误统计
- **print.go**：报告生成逻辑，生成HTML格式的报告
- **struct.go**：数据结构定义
- **tools.go**：工具函数，包括CSV文件读取、BOM文件读取、统计计算等

## 12. 注意事项

1. **输入数据质量**：确保输入数据完整且格式正确
2. **输出目录权限**：确保输出目录有写入权限
3. **参数设置**：根据实际情况提供必要的参数
4. **性能考虑**：处理大量数据时，注意系统资源使用

## 13. 联系方式

如有问题或建议，请联系：

- 作者：Wang Yaoshen
- 邮箱：wangyaoshen@zhonghegene.com

---

**版本**：1.4.0
**最后更新**：2026-03-16