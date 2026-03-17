# fastq_splitter 使用指南

## 1. 功能介绍

fastq_splitter 是一个用于根据靶标序列拆分FASTQ文件的工具，能够将测序数据按照样本进行分类，方便后续的分析处理。

### 主要功能

- **基于靶标序列拆分**：根据靶标序列将FASTQ文件拆分为不同样本
- **模糊匹配**：支持一定程度的序列差异
- **批量处理**：支持批量处理多个FASTQ文件
- **详细报告**：生成拆分统计报告
- **可视化支持**：生成可视化数据

## 2. 安装方法

### 前提条件
- Go 1.16 或更高版本
- 相关依赖库
- 可选：minimap2（用于序列比对）

### 编译步骤

```bash
# 进入项目目录
cd /path/to/SynthesisAnalyzer

# 编译fastq_splitter
go build -o bin/fastq_splitter cmd/fastq_splitter/main.go

# 或直接运行
go run cmd/fastq_splitter/main.go [参数]
```

### 依赖安装

```bash
# 运行安装脚本
chmod +x cmd/fastq_splitter/install_deps.sh
./cmd/fastq_splitter/install_deps.sh
```

## 3. 使用方法

### 基本用法

```bash
# 基本用法
fastq_splitter -i <Excel文件> -o <输出目录> -fq <Fastq目录>

# 示例
fastq_splitter -i samples.xlsx -o ./output -fq ./fastq

# 自动确定输出目录和fastq目录
fastq_splitter -i samples.xlsx

# 完整参数
fastq_splitter -i <Excel文件> [-o <输出目录>] [-fq <Fastq目录>] [-m <最小重叠长度>]
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-i` | Excel文件，包含样本信息和靶标序列 | 必需 |
| `-o` | 输出目录，默认使用Excel文件名（不含.xlsx后缀） | Excel文件名 |
| `-fq` | Fastq文件目录，默认从同目录下的*path.txt文件读取 | 从*path.txt读取 |
| `-m` | 最小重叠长度 | 30 |

## 4. 输入文件格式

### Excel文件格式

Excel文件需要包含以下列：

| 列名 | 说明 |
|------|------|
| 样品名称 | 样本的名称 |
| 靶标序列 | 靶标部分的序列 |
| 合成序列 | 合成部分的序列 |
| 后靶标 | 后靶标部分的序列 |
| 路径-R1 | R1 FASTQ文件路径 |
| 路径-R2 | R2 FASTQ文件路径 |

### path.txt文件格式

当未指定`-fq`参数时，工具会查找与Excel文件同目录的*path.txt文件（大小写不敏感），文件内容为：

- G99模式：`FT\d+`格式，如`FT100120704`
- Novo模式：`oss://novo-medical-customer-tj/...`格式，如`oss://novo-medical-customer-tj/CYB24030020/20260311_222613_23JNYHLT4-sanwen-CYB24030020_Result`

**示例Excel文件内容**：

| 样品名称 | 靶标序列 | 合成序列 | 后靶标 | 路径-R1 | 路径-R2 |
|----------|----------|----------|--------|---------|---------|
| Sample1  | ATGC...  | CGTA...  | GCAT...| sample1_R1.fastq.gz | sample1_R2.fastq.gz |
| Sample2  | ATGC...  | CGTA...  | GCAT...| sample2_R1.fastq.gz | sample2_R2.fastq.gz |

### FASTQ文件要求

- 支持.gz压缩的FASTQ文件
- 支持单端和双端测序数据
- 文件名建议包含样本信息

## 5. 自动目录检测

### 自动输出目录检测

当未指定`-o`参数时，工具会：
1. 提取Excel文件名（不含.xlsx后缀）
2. 使用该名称作为输出目录

### 自动fastq目录检测

当未指定`-fq`参数时，工具会：
1. 查找与Excel文件同目录的*path.txt文件（大小写不敏感）
2. 读取文件内容作为$fqBatch
3. 根据$fqBatch格式确定模式：
   - **G99模式**：$fqBatch为"FT\d+"格式，设置为`/data2/wangyaoshen/Sequencing_data/G99/R21007100240139/$fqBatch/L01`
   - **Novo模式**：$fqBatch为"oss://novo-medical-customer-tj/..."格式，提取最后一个目录作为$fqBatch，设置为`/data2/wangyaoshen/novo-medical-customer-tj/$CYB/$fqBatch/Rawdata`

## 6. 测序时间获取

工具会在拆分报告中添加测序时间信息：

- **G99模式**：解析fastq目录下的version.json文件中的"DateTime"字段
- **Novo模式**：提取目录名中的日期部分（前8个字符）

## 7. 输出文件说明

### 主要输出文件

1. **拆分的FASTQ文件**：
   - 按照样本分类的FASTQ文件
   - 位于输出目录的对应样本子目录中

2. **拆分报告**：
   - `split_report.csv`：详细的拆分统计报告
   - 包含每个样本的读取数、匹配率等信息

3. **可视化数据**：
   - `match_quality.csv`：匹配质量统计
   - `target_coverage.csv`：靶标覆盖度统计

4. **日志文件**：
   - `split.log`：拆分过程的详细日志

## 6. 匹配算法

### 精确匹配

- 完全匹配靶标序列
- 速度快，准确性高
- 适用于靶标序列明确的情况

### 模糊匹配

- 允许一定数量的错配
- 使用编辑距离算法
- 适用于靶标序列可能存在变异的情况

### 算法流程

1. 读取靶标序列
2. 读取FASTQ文件
3. 对每个序列进行匹配
4. 根据匹配结果分配到对应样本
5. 生成统计报告

## 7. 性能优化

### 已实现的优化

1. **并发处理**：使用goroutine并发处理多个FASTQ文件
2. **内存优化**：流式处理FASTQ文件，减少内存使用
3. **I/O优化**：批量写入数据，减少I/O操作
4. **索引优化**：使用高效的序列索引结构

### 性能建议

- 对于大量数据，建议增加并行处理数
- 对于模糊匹配，适当调整模糊度参数
- 确保有足够的磁盘空间用于输出文件

## 8. 故障排除

### 常见错误及解决方法

1. **靶标文件格式错误**
   - 错误信息：`Invalid target file format`
   - 解决方法：确保靶标文件格式正确，每行包含样本名称和靶标序列

2. **FASTQ文件读取失败**
   - 错误信息：`Failed to read FASTQ file`
   - 解决方法：确保FASTQ文件格式正确，没有损坏

3. **内存不足**
   - 错误信息：`out of memory`
   - 解决方法：减少并行处理数，或增加系统内存

4. **匹配率低**
   - 问题：拆分后的匹配率低
   - 解决方法：调整匹配模式和模糊度参数，检查靶标序列是否正确

## 9. 示例分析流程

### 步骤1：准备输入文件

1. 准备FASTQ文件，放在输入目录中
2. 创建靶标文件，包含样本和靶标序列

### 步骤2：运行拆分

```bash
# 运行fastq_splitter
fastq_splitter -i ./raw_data -o ./split_results -t targets.txt -m fuzzy -f 2

# 查看拆分进度
# 拆分完成后会显示统计信息
```

### 步骤3：查看结果

```bash
# 查看输出目录
ls ./split_results/

# 查看拆分报告
cat ./split_results/split_report.csv

# 查看每个样本的FASTQ文件
ls ./split_results/Sample1/
```

### 步骤4：分析结果

1. **拆分报告**：了解每个样本的读取数和匹配率
2. **匹配质量**：分析匹配的质量分布
3. **靶标覆盖度**：分析靶标的覆盖情况

## 10. 高级功能

### 批量处理

工具支持批量处理多个FASTQ文件，自动识别输入目录中的所有FASTQ文件并进行拆分。

### 双端测序支持

支持双端测序数据的拆分，需要同时提供R1和R2文件。

### 自定义匹配参数

可以根据实际情况调整匹配模式和模糊度参数，以获得最佳的拆分效果。

## 11. 代码结构

### 主要文件

- **main.go**：主入口，处理命令行参数
- **split.go**：核心拆分逻辑
- **match.go**：序列匹配算法
- **fuzzy.go**：模糊匹配实现
- **report.go**：报告生成
- **visualization.go**：可视化数据生成
- **tools.go**：工具函数

### 核心流程

1. 读取命令行参数
2. 读取靶标文件
3. 查找FASTQ文件
4. 并发处理FASTQ文件
5. 生成拆分文件和报告
6. 输出结果

## 12. 注意事项

1. **靶标序列质量**：确保靶标序列准确无误
2. **FASTQ文件质量**：确保FASTQ文件质量良好，没有损坏
3. **参数设置**：根据实际情况调整匹配参数
4. **性能考虑**：处理大量数据时，注意系统资源使用

## 13. 版本历史

### v1.0.0
- 初始版本
- 支持基本的FASTQ文件拆分功能

### v1.1.0
- 增加模糊匹配功能
- 优化性能
- 修复bug

### v1.2.0
- 增加双端测序支持
- 改进报告格式
- 增加可视化功能

### v1.3.0
- 增加自动输出目录检测功能
- 增加自动fastq目录检测功能
- 增加测序时间获取和报告功能
- 支持G99和Novo两种模式
- 改进命令行参数解析

## 14. 联系方式

如有问题或建议，请联系：

- 作者：Wang Yaoshen
- 邮箱：wangyaoshen@zhonghegene.com

---

**版本**：1.3.0
**最后更新**：2026-03-17