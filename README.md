# SynthesisAnalyzer

SynthesisAnalyzer 是一个用于分析合成DNA序列质量的综合工具集，包含多个子命令，用于处理和分析测序数据。

## 项目结构

```
SynthesisAnalyzer/
├── cmd/
│   ├── bam-mut-analyzer/  # BAM文件突变分析工具
│   ├── fastq_splitter/     # FASTQ文件拆分工具
│   └── report/             # 报告生成工具
├── pkg/
│   ├── stats/              # 核心统计功能包
│   └── report/             # 报告生成功能包
├── go.mod                  # Go模块定义
├── go.sum                  # 依赖版本锁定
└── README.md               # 项目文档
```

## 子命令说明

### 1. bam-mut-analyzer

**功能**：分析BAM文件中的突变情况，包括替换、插入、缺失等突变类型的统计分析。

**主要特性**：
- 支持从Excel文件读取样本信息和参考序列
- 批量处理多个BAM文件
- 详细的突变统计分析
- 生成多种统计报告文件
- 支持自定义头尾部切除长度

**使用方法**：

```bash
# 基本用法
go run cmd/bam-mut-analyzer/main.go -d <输入目录> -o <输出目录>

# 使用Excel文件指定样本顺序和参考序列
go run cmd/bam-mut-analyzer/main.go -i <Excel文件> -d <输入目录> -o <输出目录>

# 自定义参数
go run cmd/bam-mut-analyzer/main.go -i <Excel文件> -d <输入目录> -o <输出目录> -head <头切除长度> -tail <尾切除长度>
```

**参数说明**：
- `-d`：输入目录，包含样本子目录
- `-o`：输出目录，默认输入目录/mutation_stats
- `-i`：可选参数，输入Excel文件，包含样本顺序和参考序列
- `-head`：头切除长度，默认27
- `-tail`：尾切除长度，默认20
- `-max-sub`：最大替换个数阈值，用于定义比对良好reads，默认5
- `-n`：N-mer 统计的 N 值，默认4

**Excel文件格式**：
Excel文件需要包含以下列：
- 样品名称：样本的名称
- 靶标序列：靶标部分的序列
- 合成序列：合成部分的序列
- 后靶标：后靶标部分的序列

**输出文件**：
- `total_mutation_stats.csv`：总突变统计
- `read_type_by_sample.csv`：样本read类型统计
- `read_type_summary.csv`：汇总read类型统计
- `summary_by_sample.csv`：样本汇总
- `mutation_spectrum.csv`：突变频谱
- `mutation_analysis.csv`：突变分析
- `insert_length_distribution.csv`：插入长度分布
- `delete_length_distribution.csv`：缺失长度分布
- `alignment_and_mutation_rates.csv`：比对率和变异比例
- `base_mutation_counts.csv`：碱基维度变异统计
- `single_base_deletion_stats.csv`：单碱基缺失统计
- `insertion_sequence_stats.csv`：插入序列统计
- `total_single_base_deletion_positions.csv`：汇总缺失位置统计
- `deletion_position_heatmap.csv`：缺失位置热力图数据
- `read_subtype_stats.csv`：细分类统计
- `read_subtype_combinations.csv`：细分类组合统计
- `substitution_stats.csv`：替换突变统计

### 2. fastq_splitter

**功能**：根据靶标序列拆分FASTQ文件，将测序数据按照样本进行分类。

**主要特性**：
- 支持基于靶标序列的快速拆分
- 支持模糊匹配
- 生成详细的拆分报告
- 支持可视化结果
- 自动确定输出目录（当-o未定义时）
- 自动确定fastq目录（当-fq未定义时）
- 在拆分报告中添加测序时间信息

**使用方法**：

```bash
# 基本用法
go run cmd/fastq_splitter/main.go -i <Excel文件> -o <输出目录> -fq <Fastq目录>

# 自动确定输出目录和fastq目录
go run cmd/fastq_splitter/main.go -i <Excel文件>
```

**参数说明**：
- `-i`：输入Excel文件，包含样本信息和靶标序列
- `-o`：输出目录，默认使用Excel文件名（不含.xlsx后缀）
- `-fq`：Fastq目录，默认从同目录下的*path.txt文件读取
- `-m`：最小重叠长度，默认30

**自动fastq目录确定逻辑**：
1. 查找与Excel文件同目录的*path.txt文件（大小写不敏感）
2. 读取文件内容作为$fqBatch
3. 根据$fqBatch格式确定模式：
   - G99模式：$fqBatch为"FT\d+"格式，设置为`/data2/wangyaoshen/Sequencing_data/G99/R21007100240139/$fqBatch/L01`
   - Novo模式：$fqBatch为"oss://novo-medical-customer-tj/..."格式，提取最后一个目录作为$fqBatch，设置为`/data2/wangyaoshen/novo-medical-customer-tj/$CYB/$fqBatch/Rawdata`

**测序时间获取逻辑**：
- G99模式：解析fastq目录下的version.json文件中的"DateTime"字段
- Novo模式：提取目录名中的日期部分（前8个字符）

### 3. report

**功能**：生成分析报告，汇总各个工具的分析结果。

**主要特性**：
- 支持从split_summary.txt读取总处理reads数和测序时间
- 生成详细的HTML分析报告
- 支持从BOM.xlsx文件直接读取孔位信息（使用excelize库）
- 支持突变统计数据的分析和展示
- 提供默认配置，无需指定输入文件即可生成报告

**使用方法**：

```bash
# 基本用法（使用默认配置）
go run cmd/report/main.go

# 带输入文件的用法
go run cmd/report/main.go -i <输入文件>

# 带BOM文件的用法
go run cmd/report/main.go -b <BOM文件> -m <mutation_stats目录>

# 完整用法
go run cmd/report/main.go -i <输入文件> -o <输出文件> -b <BOM文件> -m <mutation_stats目录>
```

**参数说明**：
- `-i`：输入JSON文件（可选，不指定时使用默认配置）
- `-o`：输出HTML文件（可选，默认输出到stdout）
- `-b`：BOM.xlsx文件，用于获取孔位信息（必填）
- `-m`：mutation_stats目录，用于获取突变统计数据（必填）
- `-embed-image`：是否将图表以Base64编码嵌入HTML（默认false）
- `-use-go-echarts`：是否使用go-echarts生成图表（默认false）
- `-c`：配置文件路径（可选）
- `-template`：报告模板名称（默认：default）
- `-log-level`：日志级别（可选：debug, info, warn, error）

**默认值**：
- 报告标题：TIESyno-96合成仪下机报告
- 合成工艺版本：V3.0
- SEC1工艺版本：SECV1.0

## 安装方法

### 前提条件
- Go 1.16 或更高版本
- 相关依赖库

### 安装步骤

1. **克隆仓库**

```bash
git clone https://github.com/yourusername/SynthesisAnalyzer.git
cd SynthesisAnalyzer
```

2. **安装依赖**

```bash
go mod download
```

3. **编译工具**

```bash
# 编译 bam-mut-analyzer
go build -o bin/bam-mut-analyzer cmd/bam-mut-analyzer/main.go

# 编译 fastq_splitter
go build -o bin/fastq_splitter cmd/fastq_splitter/main.go

# 编译 report
go build -o bin/report cmd/report/main.go
```

## 工作流程

1. **数据准备**：准备FASTQ文件和Excel文件
2. **FASTQ拆分**：使用fastq_splitter根据靶标序列拆分FASTQ文件
3. **BAM分析**：使用bam-mut-analyzer分析拆分后的BAM文件
4. **报告生成**：使用report生成综合分析报告

### 完整工作流程示例

```bash
# 1. 使用fastq_splitter拆分FASTQ文件
fastq_splitter samples.xlsx ./output ./fastq

# 2. 使用bam-mut-analyzer分析BAM文件
bam-mut-analyzer -d ./output -i samples.xlsx

# 3. 使用report生成报告
report -i ./output -o ./reports
```

## 示例用法

### 示例1：使用bam-mut-analyzer分析BAM文件

```bash
# 使用Excel文件指定样本信息
./bin/bam-mut-analyzer -i samples.xlsx -d ./bam_files -o ./results

# 查看结果
ls ./results/
```

### 示例2：使用fastq_splitter拆分FASTQ文件

```bash
# 拆分FASTQ文件
./bin/fastq_splitter -i ./raw_data -o ./split_results -t targets.txt

# 查看拆分结果
ls ./split_results/
```

## 技术说明

### 核心算法

1. **BAM文件分析**：
   - 使用SAM/BAM文件格式解析
   - 基于MD标签和CIGAR字符串分析突变
   - 并发处理多个BAM文件

2. **FASTQ文件拆分**：
   - 基于靶标序列的匹配
   - 支持模糊匹配算法
   - 高效的序列比对

3. **统计分析**：
   - 详细的突变类型统计
   - 长度分布分析
   - 位置分布分析

### 性能优化

- **并发处理**：使用goroutine并发处理多个文件
- **内存优化**：合理使用内存，避免内存泄漏
- **I/O优化**：减少I/O操作，提高处理速度

## 注意事项

1. **输入文件格式**：确保BAM文件格式正确，FASTQ文件质量良好
2. **参考序列**：使用Excel文件提供准确的参考序列
3. **参数设置**：根据实际情况调整头尾部切除长度等参数
4. **性能考虑**：处理大量数据时，注意内存使用情况

## 故障排除

### 常见问题

1. **Excel文件读取失败**：
   - 检查Excel文件格式是否正确
   - 确保包含所有必需的列

2. **BAM文件分析失败**：
   - 检查BAM文件是否完整
   - 确保BAM文件索引文件(.bai)存在

3. **内存不足**：
   - 减少并发处理的文件数量
   - 增加系统内存

## 贡献指南

欢迎贡献代码和提出建议！请按照以下步骤：

1. Fork 仓库
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 许可证

本项目采用 MIT 许可证。详见 LICENSE 文件。

## 联系方式

如有问题或建议，请联系：

- 作者：Wang Yaoshen
- 邮箱：wangyaoshen@zhonghegene.com

---

**版本**：1.0.0
**最后更新**：2026-03-20