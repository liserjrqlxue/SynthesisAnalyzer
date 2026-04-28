package synthesis

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

type SynthesisAnalyzer struct {
	// 配置参数
	config Config

	// 参考序列信息
	references  []Reference
	barcodeMap  map[string]int     // barcode->reference index
	bloomFilter *bloom.BloomFilter // 添加Bloom过滤器

	// 统计结果
	results     []PositionStats
	resultMutex sync.RWMutex
}

type Config struct {
	InputR1       string
	InputR2       string
	ReferenceFile string
	OutputDir     string
	Threads       int
	MemoryGB      int

	// Barcode相关
	BarcodeStartLen int // 头部barcode长度
	BarcodeEndLen   int // 尾部barcode长度
	BarcodeMismatch int // 允许的错配数

	// Fastp参数
	MergeLen      int // 合并后长度
	QualityCutoff int
}

type Reference struct {
	ID           string
	Sequence     string
	Length       int
	BarcodeStart string
	BarcodeEnd   string
}

type PositionStats struct {
	ReferenceID    string
	Position       int
	TotalReads     int64
	MatchCount     int64
	MismatchCount  int64
	DeletionCount  int64
	InsertionCount int64
}
