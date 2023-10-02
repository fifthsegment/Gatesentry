package gatesentryf

type GSFilterLog struct {
	Content  string
	Filtered bool
	Score    int
}

func NewLogItem(content string) *GSFilterLog {
	return &GSFilterLog{
		Content:  content,
		Filtered: false,
		Score:    0,
	}
}

func (L *GSFilterLog) Add() {
	// R.Logger.Info( strconv.Itoa( (R.MemLogSz) ) )
	if len(R.MemLog) >= R.MemLogSz {
		R.MemLog = R.MemLog[:0]
	}
	R.MemLog = append(R.MemLog, *L)
}
