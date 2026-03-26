package stats

// ========== 新增：位置详细统计结构 ==========
type PositionDetail struct {
	Base                byte // ACGTN
	Depth               int
	MatchPure           int
	MatchWithIns        int
	MismatchPure        int
	MismatchWithIns     int
	Insertion           int
	Deletion            int
	Del1                int
	PerfectReadsCount   int
	PerfectUptoPosCount int
	AlignedSum          int // 覆盖该位置的所有样品的 aligned reads 之和
	TotalSum            int // 覆盖该位置的所有样品的 total reads 之和

	NCorrect  int
	N1Correct int
}

// InsertSubtype 插入子类型（多条插入）
type InsertSubtype struct {
	Insertions []InsertionInfo // 每条插入的信息
}

// InsertionInfo 单条插入信息
type InsertionInfo struct {
	Length  int              // 插入长度
	Bases   string           // 插入的碱基序列
	Subtype InsertionSubtype // 细分类
}

// DeleteSubtype 缺失子类型
type DeleteSubtype struct {
	Deletions []DeletionInfo // 每条缺失的信息
}

// DeletionInfo 单条缺失信息
type DeletionInfo struct {
	Length   int             // 缺失长度
	Bases    string          // 缺失的碱基序列（如果长度为1）
	Position int             // 缺失位置（1-based）
	Subtype  DeletionSubtype // 细分类
}

// Mutation 表示一个碱基突变
type Mutation struct {
	Position int
	Ref      string
	Alt      string
	Subtype  SubstitutionSubtype
}

// 新增：替换信息结构（用于记录替换细分类）
type SubstitutionInfo struct {
	Position int
	Ref      string
	Alt      string
	Subtype  SubstitutionSubtype
}

// ReadDetailedInfo 详细的read信息
type ReadDetailedInfo struct {
	MainType       ReadType
	InsertSub      *InsertSubtype
	DeleteSub      *DeleteSubtype
	Mutations      []Mutation // 该read中的所有突变，兼容替换细分类列表
	SubtypeTags    []string   // 该read的所有细分类标签（去重后排序用）
	CombinationKey string     // 细分类组合唯一键（排序后拼接）
}

// 定义累加器结构
type SumPct struct {
	SumGood    float64
	SumAligned float64
	SumTotal   float64
	Count      int
}
