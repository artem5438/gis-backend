package parcels

import (
	"strconv"
	"strings"
)

func ruFloat(value float64, frac int) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	raw := strconv.FormatFloat(value, 'f', frac, 64)
	parts := strings.SplitN(raw, ".", 2)
	intPart := parts[0]
	grouped := make([]byte, 0, len(intPart)+len(intPart)/3)
	for index := 0; index < len(intPart); index++ {
		if index > 0 && (len(intPart)-index)%3 == 0 {
			grouped = append(grouped, ' ')
		}
		grouped = append(grouped, intPart[index])
	}
	text := sign + string(grouped)
	if frac == 0 || len(parts) < 2 {
		return text
	}
	tail := strings.TrimRight(parts[1], "0")
	if tail == "" {
		return text
	}
	return text + "," + tail
}
