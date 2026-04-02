package cfg

import (
	"strings"

	"github.com/biogo/hts/sam"
)

func GetMD(read *sam.Record) (mdStr string, hasMD bool) {
	var mdTag sam.Aux
	mdTag, hasMD = read.Tag([]byte{'M', 'D'})
	if hasMD {
		mdStr = mdTag.String()
		if len(mdStr) > 5 && mdStr[4] == ':' {
			mdStr = mdStr[5:]
		} else {
			mdStr = strings.TrimPrefix(mdStr, "MD:Z:")
		}
	}

	return
}
