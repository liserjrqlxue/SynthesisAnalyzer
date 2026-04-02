package splitter

import (
	"regexp"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"

	"SynthesisAnalyzer/pkg/cfg"
)

// 合并文件与样品的关系
type MergedFileInfo struct {
	FilePath     string
	SampleNames  []string
	Samples      []*cfg.Sample // 只包含该文件对应的样品
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
	config    *cfg.Config
	samples   []*cfg.Sample
	fileMap   map[string][]*cfg.Sample // merged文件 -> 样品列表
	sampleMap map[string]*cfg.Sample   // 样品名称 -> 样品信息

	// 新的结构：合并文件信息
	mergedFiles   []*MergedFileInfo
	mergedFileMap map[string]*MergedFileInfo // 文件路径 -> 文件信息

	// 索引结构
	forwardIndex map[string]*cfg.Sample // 正向序列 -> 样本
	reverseIndex map[string]*cfg.Sample // 反向序列 -> 样本
	bloomFilter  *bloom.BloomFilter

	// 每个合并文件独立的匹配器
	fileMatchers map[string]*FileMatcher

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
func NewEnhancedSplitter(config *cfg.Config) *EnhancedSplitter {
	return &EnhancedSplitter{
		config:        config,
		samples:       []*cfg.Sample{},
		sampleMap:     make(map[string]*cfg.Sample),
		fileMap:       make(map[string][]*cfg.Sample),
		forwardIndex:  make(map[string]*cfg.Sample),
		reverseIndex:  make(map[string]*cfg.Sample),
		mergedFiles:   []*MergedFileInfo{},
		mergedFileMap: make(map[string]*MergedFileInfo),
		fileMatchers:  make(map[string]*FileMatcher),
	}
}

// 文件匹配器（每个合并文件一个）
type FileMatcher struct {
	fileInfo     *MergedFileInfo
	forwardRegex map[string]*regexp.Regexp // 样品名称 -> 正向正则
	reverseRegex map[string]*regexp.Regexp // 样品名称 -> 反向正则
	useRC        bool
}

// 解析单个read时使用的临时结构体
type readErrorFlags struct {
	hasMismatch  bool
	hasInsertion bool
	hasDeletion  bool
}

// 比对结果结构
type AlignmentResult struct {
	Sample    *cfg.Sample
	Alignment *cfg.SampleAlignment
	Error     error
}
