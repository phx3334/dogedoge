package pkg

import (
	"strconv"
)


// mustParseUint 将字符串解析为 uint，失败返回 0
func MustParseUint(s string) uint {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}
