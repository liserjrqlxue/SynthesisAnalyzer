package main

// ReadType 表示read的类型
type ReadType int

const (
	ReadTypeMatch              ReadType = iota // 匹配
	ReadTypeInsert                             // 插入
	ReadTypeDelete                             // 缺失
	ReadTypeSubstitution                       // 替换
	ReadTypeInsertDelete                       // 插入+缺失
	ReadTypeInsertSubstitution                 // 插入+替换
	ReadTypeDeleteSubstitution                 // 缺失+替换
	ReadTypeAll                                // 插入+缺失+替换
)

// ReadTypeNames ReadType对应的名称
var ReadTypeNames = map[ReadType]string{
	ReadTypeMatch:              "匹配",
	ReadTypeInsert:             "插入",
	ReadTypeDelete:             "缺失",
	ReadTypeSubstitution:       "替换",
	ReadTypeInsertDelete:       "插入+缺失",
	ReadTypeInsertSubstitution: "插入+替换",
	ReadTypeDeleteSubstitution: "缺失+替换",
	ReadTypeAll:                "插入+缺失+替换",
}

// 新增：缺失细分类
type DeletionSubtype int

const (
	Del1 DeletionSubtype = iota // 长度1
	Del2                        // 长度>1
)

// 新增：插入细分类
type InsertionSubtype int

const (
	Dup1   InsertionSubtype = iota // 长度1，与-1或+1碱基一致
	Dup2                           // 长度2，插入序列一致且与-1或+1碱基一致
	DupDup                         // 长度2，插入序列不一致，第一位与-1一致，第二位与+1一致
	Ins1                           // 长度1，非Dup1
	Ins2                           // 长度2，非Dup2/DupDup
	Ins3                           // 长度>2
)

// 新增：替换细分类
type SubstitutionSubtype int

const (
	DupDel   SubstitutionSubtype = iota // 与-1位相同
	DelDup                              // 与+1位相同
	Mismatch                            // 其他
)
