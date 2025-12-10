package synthesis

import (
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

type SynthesisAnalyzer struct {
	// 配置参数
	config Config

	// 参考序列信息
	references []Reference
	barcodeMap map[string]int // barcode->reference index

	// 统计结果
	results     []PositionStats
	resultMutex sync.RWMutex

	// 工作池
	workPool chan *ReadBatch
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

type OptimizedConfig struct {
	// 内存优化
	UseMemoryPool    bool
	PoolBufferSize   int // 64KB
	MaxMemoryUsageGB int

	// IO优化
	UseDirectIO bool
	ReadAheadKB int // 4096
	UseAsyncIO  bool

	// CPU优化
	ThreadAffinity bool
	NUMAOptimized  bool
	UseSIMD        bool // 启用AVX2/SSE

	// 算法优化
	BatchSize       int  // 10000 reads per batch
	BloomFilterBits int  // 用于快速barcode过滤
	UseApproxMatch  bool // 使用近似匹配加速
}

func (a *SynthesisAnalyzer) applyOptimizations() {
	// 设置Go运行时参数
	runtime.GOMAXPROCS(a.config.Threads)
	debug.SetGCPercent(30) // 降低GC频率

	// 大页内存支持
	if a.config.MemoryGB > 64 {
		a.enableHugePages()
	}

	// 设置文件系统预读
	a.setReadAhead()
}

type Monitor struct {
	startTime time.Time
	metrics   map[string]interface{}
	mutex     sync.RWMutex
}

func (m *Monitor) TrackPhase(phase string, fn func() error) error {
	start := time.Now()
	log.Printf("开始阶段: %s", phase)

	err := fn()

	duration := time.Since(start)
	m.mutex.Lock()
	m.metrics[phase+"_duration"] = duration
	m.metrics[phase+"_success"] = err == nil
	m.mutex.Unlock()

	if err != nil {
		log.Printf("阶段 %s 失败: %v", phase, err)
	} else {
		log.Printf("阶段 %s 完成, 耗时: %v", phase, duration)
	}

	return err
}
