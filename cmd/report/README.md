# report 使用指南

## 1. 功能介绍

report 是一个用于生成分析报告的工具，能够汇总 fastq_splitter 和 bam-mut-analyzer 的分析结果，生成综合报告。

### 主要功能

- **报告生成**：生成综合分析报告
- **结果汇总**：汇总 fastq_splitter 和 bam-mut-analyzer 的结果
- **数据可视化**：生成可视化图表数据
- **自定义报告**：支持自定义报告内容和格式

### 注意事项

**当前状态**：report 工具目前是未完成品，正在开发中。

## 2. 安装方法

### 前提条件
- Go 1.16 或更高版本
- 相关依赖库

### 编译步骤

```bash
# 进入项目目录
cd /path/to/SynthesisAnalyzer

# 编译report
go build -o bin/report cmd/report/main.go

# 或直接运行
go run cmd/report/main.go [参数]
```

## 3. 使用方法

### 基本用法

```bash
# 基本用法
report -i <输入目录> -o <输出目录>

# 输入目录应该包含fastq_splitter和bam-mut-analyzer的结果
# 示例
report -i ./output -o ./reports
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-i` | 输入目录，包含fastq_splitter和bam-mut-analyzer的结果 | 必需 |
| `-o` | 输出目录 | 必需 |
| `-t` | 报告类型（summary/detail） | summary |
| `-f` | 输出格式（html/csv/pdf） | html |
| `-n` | 报告名称 | report |
| `-c` | 配置文件 | 可选 |

## 4. 输入文件格式

### 输入目录结构

输入目录应该包含 fastq_splitter 和 bam-mut-analyzer 生成的结果文件：

```
input_dir/
├── fastq_splitter_results/  # fastq_splitter的结果
│   ├── split_report.csv
│   └── ...
├── mutation_stats/          # bam-mut-analyzer的结果
│   ├── total_mutation_stats.csv
│   ├── read_type_summary.csv
│   └── ...
└── ...
```

### 支持的输入文件

- **fastq_splitter**：split_report.csv等
- **bam-mut-analyzer**：
  - total_mutation_stats.csv
  - read_type_summary.csv（用于子类型分类）
  - read_type_by_sample.csv（用于收率统计）
  - 其他相关统计文件

## 5. 输出文件说明

### 主要输出文件

1. **综合报告**：
   - `report.html`：HTML格式的综合报告
   - `report.csv`：CSV格式的报告数据
   - `report.pdf`：PDF格式的报告（如果支持）

2. **可视化数据**：
   - `charts/`：包含各种图表的目录
   - `tables/`：包含各种表格的目录

3. **汇总数据**：
   - `summary.csv`：汇总统计数据
   - `details.csv`：详细统计数据

## 6. 报告内容

### 摘要报告（summary）

- **总体统计**：总读取数、突变率等
- **主要发现**：关键突变类型和频率
- **质量评估**：数据质量评估
- **收率统计**：基于匹配/GoodAlignedReads的收率信息
- **建议**：基于分析结果的建议

### 详细报告（detail）

- **样本统计**：每个样本的详细统计，包括收率信息
- **突变分析**：详细的突变类型分析，使用子类型分类
- **长度分布**：插入和缺失的长度分布
- **位置分布**：突变的位置分布
- **质量控制**：质量控制指标

### 错误类型分类

report工具现在使用子类型分类来统计合成错误，从bam-mut-analyzer生成的`mutation_stats/read_type_summary.csv`的细分类统计第三列获取：

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

### 收率统计

收率信息从bam-mut-analyzer生成的`mutation_stats/read_type_by_sample.csv`的匹配/GoodAlignedReads获取每个样品的，然后计算各种统计值。

## 7. 可视化功能

### 支持的图表类型

- **柱状图**：显示不同类型的分布
- **折线图**：显示趋势变化
- **热力图**：显示位置分布
- **饼图**：显示比例分布
- **散点图**：显示相关性

### 可视化数据格式

- **CSV**：用于生成图表的数据
- **JSON**：用于交互式图表
- **SVG**：静态图表

## 8. 性能优化

### 已实现的优化

1. **并行处理**：使用goroutine并发处理数据
2. **内存优化**：流式处理数据，减少内存使用
3. **I/O优化**：批量读取和写入数据

### 性能建议

- 对于大量数据，建议使用摘要报告
- 确保有足够的磁盘空间用于输出文件
- 对于复杂报告，可能需要较长的处理时间

## 9. 故障排除

### 常见错误及解决方法

1. **输入文件格式错误**
   - 错误信息：`Invalid input file format`
   - 解决方法：确保输入文件格式正确，符合工具要求

2. **数据读取失败**
   - 错误信息：`Failed to read input data`
   - 解决方法：确保输入目录存在，包含必要的文件

3. **内存不足**
   - 错误信息：`out of memory`
   - 解决方法：减少报告类型的复杂度，或增加系统内存

4. **报告生成失败**
   - 错误信息：`Failed to generate report`
   - 解决方法：检查输出目录权限，确保有足够的空间

## 10. 示例分析流程

### 步骤1：准备输入数据

1. 运行 fastq_splitter 拆分FASTQ文件
2. 运行 bam-mut-analyzer 分析BAM文件
3. 组织输入目录结构

### 步骤2：运行报告生成

```bash
# 运行report
report -i ./output -o ./reports -t detail -f html

# 查看报告生成进度
# 生成完成后会显示报告路径
```

### 步骤3：查看报告

```bash
# 查看输出目录
ls ./reports/

# 打开HTML报告
open ./reports/report.html

# 查看CSV数据
cat ./reports/summary.csv
```

### 步骤4：分析报告

1. **摘要部分**：了解总体情况
2. **详细部分**：深入分析具体数据
3. **可视化部分**：通过图表直观了解数据
4. **建议部分**：根据分析结果采取相应措施

## 11. 高级功能

### 自定义报告

可以通过配置文件自定义报告内容和格式：

```yaml
# 配置文件示例
report:
  title: "Synthesis Analysis Report"
  sections:
    - name: "Summary"
      enabled: true
    - name: "Mutation Analysis"
      enabled: true
    - name: "Quality Control"
      enabled: true
  charts:
    - type: "bar"
      data: "mutation_types"
      title: "Mutation Types"
```

### 批量报告

支持批量生成多个报告，例如按样本生成报告：

```bash
report -i ./output -o ./reports -t detail -f html --batch sample
```

### 集成其他工具

可以集成其他分析工具的结果，只需按照规定的格式组织输入文件。

## 12. 代码结构

### 主要文件

- **main.go**：主入口，处理命令行参数
- **print.go**：报告生成逻辑
- **struct.go**：数据结构定义
- **tools.go**：工具函数

### 核心流程

1. 读取命令行参数
2. 读取输入数据（fastq_splitter和bam-mut-analyzer的结果）
3. 从read_type_summary.csv获取子类型分类数据
4. 从read_type_by_sample.csv获取收率统计数据
5. 处理和汇总数据
6. 生成报告
7. 输出结果

## 13. 注意事项

1. **输入数据质量**：确保输入数据完整且格式正确
2. **输出目录权限**：确保输出目录有写入权限
3. **参数设置**：根据实际情况调整报告类型和格式
4. **性能考虑**：处理大量数据时，注意系统资源使用
5. **开发状态**：report工具目前是未完成品，功能可能不完整

## 14. 版本历史

### v1.0.0
- 初始版本
- 支持基本的报告生成功能

### v1.1.0
- 增加可视化功能
- 优化报告格式
- 修复bug

### v1.2.0
- 增加自定义报告功能
- 改进数据处理
- 增加批量报告支持

## 15. 联系方式

如有问题或建议，请联系：

- 作者：Wang Yaoshen
- 邮箱：wangyaoshen@zhonghegene.com

---

**版本**：1.2.0
**最后更新**：2026-03-13