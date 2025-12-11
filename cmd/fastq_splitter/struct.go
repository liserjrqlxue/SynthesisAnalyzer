package main

import (
	"regexp"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// 样品信息
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

	// 运行选项
	SkipExisting  bool // 是否跳过已存在的文件
	Compression   bool // 是否压缩输出
	CompressLevel int  // 压缩级别 1-9，默认6
	CleanupTemp   bool // 是否清理临时文件
}

// 更新后的拆分处理器
type RegexpSplitter struct {
	config  *Config
	samples []*SampleInfo
	fileMap map[string][]*SampleInfo // merged文件 -> 样品列表

	// 索引结构
	forwardIndex map[string]*SampleInfo // 正向序列 -> 样本
	reverseIndex map[string]*SampleInfo // 反向序列 -> 样本
	bloomFilter  *bloom.BloomFilter

	seqRegexps []*SeqRegexp
	useRC      bool

	// 统计
	stats struct {
		totalReads     int64
		matchedReads   int64
		ambiguousReads int64
		failedReads    int64
		forwardMatches int64
		reverseMatches int64
	}
	statsMutex sync.RWMutex
}

// 创建正则表达式拆分器
func NewRegexpSplitter(config *Config) *RegexpSplitter {
	return &RegexpSplitter{
		config:       config,
		samples:      []*SampleInfo{},
		fileMap:      make(map[string][]*SampleInfo),
		forwardIndex: make(map[string]*SampleInfo),
		reverseIndex: make(map[string]*SampleInfo),
		useRC:        config.UseRC,
	}
}

// 序列正则表达式信息
type SeqRegexp struct {
	sample       *SampleInfo
	forwardRegex *regexp.Regexp
	reverseRegex *regexp.Regexp
}
