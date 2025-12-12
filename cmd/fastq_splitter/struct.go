package main

import (
	"regexp"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// 配置结构
type Config struct {
	ExcelFile string
	OutputDir string
	FastqDir  string
	Threads   int
	UseRC     bool // 是否使用反向互补

	// 匹配参数
	SearchWindow int // 搜索窗口大小（从头、从尾搜索的距离）
	Quality      int // fastp质量阈值
	MergeLen     int // 合并后长度
	// 模糊匹配参数
	AllowMismatch  int // 允许的错配数（0表示完全匹配）
	MatchThreshold int // 匹配分数阈值

	// 运行选项
	SkipExisting  bool // 是否跳过已存在的文件
	Compression   bool // 是否压缩输出
	CompressLevel int  // 压缩级别 1-9，默认6
	CleanupTemp   bool // 是否清理临时文件

	// 输出选项
	OutputMode string // "full"或"target-only"
}

// 扩展SampleInfo结构，添加提取统计
type SampleInfo struct {
	Name          string
	TargetSeq     string // 头靶标序列
	SynthesisSeq  string // 合成序列
	PostTargetSeq string // 尾靶标序列
	R1Path        string
	R2Path        string
	OutputPath    string

	// 完整参考序列（用于分析）
	FullReference string

	// 正则表达式匹配结果
	TotalReads   int
	MatchedReads int
	ForwardReads int
	ReverseReads int

	// 提取序列统计
	MinExtractedLen   int
	MaxExtractedLen   int
	TotalExtractedLen int

	// 状态信息
	MergedFile string
	DoneFile   string // 完成标签文件
	Status     string
	ReadCount  int

	// 反向互补序列（用于匹配）
	ReverseTarget     string // 头靶标反向互补
	ReversePostTarget string // 尾靶标反向互补

	MergeTime time.Time
	SplitTime time.Time
}

// 合并文件与样品的关系
type MergedFileInfo struct {
	FilePath     string
	SampleNames  []string
	Samples      []*SampleInfo // 只包含该文件对应的样品
	Status       string        // pending, processing, done
	TotalReads   int
	MatchedReads int
}

// 更新后的拆分处理器
type EnhancedSplitter struct {
	config    *Config
	samples   []*SampleInfo
	fileMap   map[string][]*SampleInfo // merged文件 -> 样品列表
	sampleMap map[string]*SampleInfo   // 样品名称 -> 样品信息

	// 新的结构：合并文件信息
	mergedFiles   []*MergedFileInfo
	mergedFileMap map[string]*MergedFileInfo // 文件路径 -> 文件信息

	// 索引结构
	forwardIndex map[string]*SampleInfo // 正向序列 -> 样本
	reverseIndex map[string]*SampleInfo // 反向序列 -> 样本
	bloomFilter  *bloom.BloomFilter

	// 每个合并文件独立的匹配器
	fileMatchers map[string]*FileMatcher
	useRC        bool

	// 全局统计
	stats      *SplitStats
	statsMutex sync.RWMutex
}

// 拆分统计
type SplitStats struct {
	totalFiles     int
	totalSamples   int
	totalReads     int64
	totalMatched   int64
	totalFailed    int64
	forwardMatches int64
	reverseMatches int64
	ambiguousReads int64
	startTime      time.Time
	endTime        time.Time
}

// 文件处理统计
type FileProcessStats struct {
	filePath     string
	totalReads   int
	matchedReads int
}

// 创建正则表达式拆分器
func NewEnhancedSplitter(config *Config) *EnhancedSplitter {
	return &EnhancedSplitter{
		config:        config,
		samples:       []*SampleInfo{},
		sampleMap:     make(map[string]*SampleInfo),
		fileMap:       make(map[string][]*SampleInfo),
		forwardIndex:  make(map[string]*SampleInfo),
		reverseIndex:  make(map[string]*SampleInfo),
		mergedFiles:   []*MergedFileInfo{},
		mergedFileMap: make(map[string]*MergedFileInfo),
		fileMatchers:  make(map[string]*FileMatcher),
		useRC:         config.UseRC,
	}
}

// 文件匹配器（每个合并文件一个）
type FileMatcher struct {
	fileInfo     *MergedFileInfo
	forwardRegex map[string]*regexp.Regexp // 样品名称 -> 正向正则
	reverseRegex map[string]*regexp.Regexp // 样品名称 -> 反向正则
	useRC        bool
}
