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

	// 比对相关配置
	Alignment AlignmentConfig

	// --overlap_len_require
	// the minimum length to detect overlapped region of PE reads. This will affect overlap analysis based PE merge, adapter trimming and correction. 30 by default. (int [=30])
	OverlapLenRequire int
}

// 合并文件与样品的关系
type MergedFileInfo struct {
	FilePath     string
	SampleNames  []string
	Samples      []*SampleInfo // 只包含该文件对应的样品
	Status       string        // pending, processing, done
	TotalReads   int
	MatchedReads int
	// fastp统计信息
	BeforeFilteringTotalReads int
	BeforeFilteringTotalBases int64
	AfterFilteringTotalReads  int
	AfterFilteringTotalBases  int64
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

	// 测序时间
	sequencingTime string
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

// 比对结果结构
type SampleAlignment struct {
	SampleName     string
	ReferenceSeq   string
	ReferenceLen   int
	PositionStats  []PositionStat
	Summary        *AlignmentSummary
	BamFile        string
	BamIndex       string
	ReadTypeCounts *ReadTypeCounts // 新增：记录各种类型的reads个数
}

type ReadTypeCounts struct {
	PerfectReads     int64 // 完全正确的reads
	MismatchOnly     int64 // 只有错配的reads
	InsertionOnly    int64 // 只有插入的reads
	DeletionOnly     int64 // 只有缺失的reads
	MixedMismatchIns int64 // 同时有错配和插入
	MixedMismatchDel int64 // 同时有错配和缺失
	MixedInsDel      int64 // 同时有插入和缺失
	AllErrors        int64 // 三种错误都有
	Other            int64 // 其他组合或无法解析的
}

// 位置统计结构
type PositionStat struct {
	Position       int
	TotalReads     int64
	MatchCount     int64
	MismatchCount  int64
	DeletionCount  int64
	InsertionCount int64
	Coverage       float64
	ErrorRate      float64
}

// 比对汇总
type AlignmentSummary struct {
	TotalReads       int64
	MappedReads      int64
	MappingRate      float64
	AverageCoverage  float64
	AverageIdentity  float64
	SynthesisSuccess float64         // 合成成功率
	ErrorPositions   []int           // 高错误率位置
	ReadTypeCounts   *ReadTypeCounts // 新增
}

// 解析单个read时使用的临时结构体
type readErrorFlags struct {
	hasMismatch  bool
	hasInsertion bool
	hasDeletion  bool
}

// 比对结果结构
type AlignmentResult struct {
	Sample    *SampleInfo
	Alignment *SampleAlignment
	Error     error
}
