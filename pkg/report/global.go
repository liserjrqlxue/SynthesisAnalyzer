package report

// 子类型分类映射（中文名称）
var subtypeMapping = map[string]string{
	"Del1":          "缺失1",
	"Del2":          "连续两个缺失",
	"Del3":          "连续三个及以上缺失",
	"MismatchA>G":   "A-G突变",
	"MismatchC>T":   "C-T突变",
	"OtherMismatch": "其余突变",
	"Dup1":          "重复插入1个",
	"Dup2":          "重复插入2个",
	"Ins1":          "插入1（与前后不同）",
	"Ins2":          "插入2（与前后不同）",
	"Ins3":          "插入3个及以上（与前后不同）",
	"DelDup":        "缺失+重复",
	"DupDel":        "重复+缺失",
}

// 错误类型顺序，按照subtypeMapping的key顺序
var errorTypes = []string{
	"Del1",
	"Del2",
	"Del3",
	"MismatchA>G",
	"MismatchC>T",
	"OtherMismatch",
	"Dup1",
	"Dup2",
	"Ins1",
	"Ins2",
	"Ins3",
	"DelDup",
	"DupDel",
}

// 错误类型详细信息映射
var errorTypeInfoMap = map[string]ErrorTypeInfo{
	"Del1":          {EN: "DEL1", CN: "缺失1", Example: "ATG>AG"},
	"Del2":          {EN: "DEL2", CN: "连续两个缺失", Example: "ATCG>AG"},
	"Del3":          {EN: "DEL3", CN: "连续三个及以上缺失", Example: "ATCGA>AA"},
	"MismatchA>G":   {EN: "Mismatch-AG", CN: "A-G突变", Example: "A>G"},
	"MismatchC>T":   {EN: "Mismatch-CT", CN: "C-T突变", Example: "C>T"},
	"OtherMismatch": {EN: "Other Mismatch", CN: "其余突变", Example: "A>T"},
	"Dup1":          {EN: "DUP1", CN: "重复插入1个", Example: "ATG>ATTG"},
	"Dup2":          {EN: "DUP2", CN: "重复插入2个", Example: "ATG>ATTTG"},
	"Ins1":          {EN: "INS1", CN: "插入1（与前后不同）", Example: "AG>ACG"},
	"Ins2":          {EN: "INS2", CN: "插入2（与前后不同）", Example: "AG>ACTG"},
	"Ins3":          {EN: "INS3", CN: "插入3个及以上（与前后不同）", Example: "AG>ACCTG"},
	"DelDup":        {EN: "DELDUP", CN: "缺失+重复", Example: "AG>GG"},
	"DupDel":        {EN: "DUPDEL", CN: "重复+缺失", Example: "AG>AA"},
}

// ErrorTypeInfo 错误类型详细信息
type ErrorTypeInfo struct {
	EN      string // 英文名称
	CN      string // 中文名称
	Example string // 示例
}
