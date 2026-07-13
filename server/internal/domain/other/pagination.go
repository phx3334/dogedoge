package other

import (
	"fake_tiktok/internal/dto/request"
)

type MySQLOption struct {
	request.PageInfo
	Order   string
	Filters map[string]interface{}
	Preload []string
}

type CursorField struct {
	Column    string
	Direction string
}

type CursorOption struct {
	request.CursorPage
	Fields []CursorField
}
