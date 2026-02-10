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
