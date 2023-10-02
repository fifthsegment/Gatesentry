package gatesentryf

import (
	"strconv"
)

var sizeKB = float64(1 << (10 * 1))
var sizeMB = float64(1 << (10 * 2)) // 2 refers to the constants ByteSize MB -- example of declaring 5 MB
var sizeGB = float64(1 << (10 * 3)) // 3 refers to the constants ByteSize GB
var sizeTB = float64(1 << (10 * 4))
var sizePB = float64(1 << (10 * 5))

func GetHumanDataSize(dataa int64) string {
	data := float64(dataa)
	//prec := 4
	if data < sizeMB {
		return strconv.FormatFloat(data/sizeKB, 'g', 5, 64) + "Kb"
	}
	if data < sizeGB {
		return strconv.FormatFloat(data/sizeMB, 'g', 5, 64) + "Mb"
	}
	if data < sizeTB {
		return strconv.FormatFloat(data/sizeGB, 'g', 5, 64) + "Gb"
	}
	if data < sizePB {
		return strconv.FormatFloat(data/sizeTB, 'g', 5, 64) + "Tb"
	}
	return ""
}
