package mysql

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"fake_tiktok/internal/domain/other"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var (
	_ interfaces.PaginateRepository = (*PaginateRepo)(nil)
)

type PaginateRepo struct {
	db *gorm.DB
}

func NewPaginateRepo(db *gorm.DB) *PaginateRepo {
	return &PaginateRepo{db: db}
}

func (r *PaginateRepo) Paginate(ctx context.Context, option other.MySQLOption, dest interface{}) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if option.Page < 1 {
		option.Page = 1
	}
	if option.PageSize < 1 {
		option.PageSize = 10
	}
	if option.Order == "" {
		option.Order = "id desc"
	}
	db := r.db.WithContext(ctx).Model(dest)
	for k, v := range option.Filters {
		db = db.Where(k+"=?", v)
	}

	db = db.Order(option.Order)

	for _, p := range option.Preload {
		db = db.Preload(p)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, err
	}
	if err := db.Offset((option.Page - 1) * option.PageSize).Limit(option.PageSize).Find(dest).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// CursorPaginate 执行游标分页查询。
// query: 基础查询（已包含 WHERE 条件，如 status='published'）
// opt: 游标分页选项，包含游标值、每页数量、排序字段
// dest: 结果切片指针，如 &[]database.Video
// 返回值: 总记录数、下一页游标（空串表示无下一页）、错误
func (r *PaginateRepo) CursorPaginate(ctx context.Context, query *gorm.DB, opt other.CursorOption, dest interface{}) (int64, string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 修复：创建新 Session，避免修改传入的 *gorm.DB 对象状态影响调用方
	query = query.Session(&gorm.Session{})

	limit := opt.Limit
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	var total int64
	// 使用 WithContext 传播超时控制，确保游标分页查询也受 context 超时约束
	if err := query.WithContext(ctx).Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return 0, "", err
	}

	if opt.Cursor != "" {
		values, err := decodeCursorValues(opt.Cursor)
		if err == nil && len(values) == len(opt.Fields) {
			query = applyCursorConditions(query, opt.Fields, values)
		}
	}

	for _, f := range opt.Fields {
		query = query.Order(f.Column + " " + strings.ToUpper(f.Direction))
	}

	query = query.Limit(limit + 1)

	if err := query.WithContext(ctx).Find(dest).Error; err != nil {
		return 0, "", err
	}

	destVal := reflect.ValueOf(dest).Elem()
	var nextCursor string
	if destVal.Len() > limit {
		lastItem := destVal.Index(limit)
		values := extractCursorValues(lastItem, opt.Fields)
		nextCursor = encodeCursorValues(values)
		destVal.SetLen(limit)
	}

	return total, nextCursor, nil
}

// applyCursorConditions 根据游标字段和值构建 WHERE 条件。
// 对于 [f1 DESC, f2 DESC] + [v1, v2]，生成：
//
//	WHERE (f1 < v1) OR (f1 = v1 AND f2 < v2)
//
// 对于 [f1 ASC, f2 DESC] + [v1, v2]，生成：
//
//	WHERE (f1 > v1) OR (f1 = v1 AND f2 < v2)
//
// 注意：当前游标分页未处理 NULL 值。
// 若游标字段值为 NULL，SQL 中 NULL < v1 和 NULL = v1 的结果均为 UNKNOWN，
// 导致该行不会被游标分页返回。建议游标字段定义中禁止 NULL。
func applyCursorConditions(query *gorm.DB, fields []other.CursorField, values []interface{}) *gorm.DB {
	var conditions []string
	var args []interface{}

	for i := 0; i < len(fields); i++ {
		op := "<"
		if strings.ToUpper(fields[i].Direction) == "ASC" {
			op = ">"
		}

		var parts []string
		var partArgs []interface{}
		for j := 0; j < i; j++ {
			parts = append(parts, fields[j].Column+" = ?")
			partArgs = append(partArgs, values[j])
		}
		parts = append(parts, fields[i].Column+" "+op+" ?")
		partArgs = append(partArgs, values[i])

		conditions = append(conditions, "("+strings.Join(parts, " AND ")+")")
		args = append(args, partArgs...)
	}

	return query.Where(strings.Join(conditions, " OR "), args...)
}

// extractCursorValues 从结构体中提取游标字段值。
// 使用 snake_case 列名转换为 PascalCase 来匹配 Go 结构体字段。
func extractCursorValues(v reflect.Value, fields []other.CursorField) []interface{} {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	var values []interface{}
	for _, f := range fields {
		goName := toPascalCase(f.Column)
		fieldVal := v.FieldByName(goName)
		if fieldVal.IsValid() {
			values = append(values, fieldVal.Interface())
		}
	}
	return values
}

// encodeCursorValues 将游标字段值编码为 base64 字符串。
func encodeCursorValues(values []interface{}) string {
	data, _ := json.Marshal(values)
	return base64.StdEncoding.EncodeToString(data)
}

// decodeCursorValues 从 base64 字符串解码游标字段值。
func decodeCursorValues(cursor string) ([]interface{}, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var values []interface{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("invalid cursor values: %w", err)
	}
	return values, nil
}

// toPascalCase 将 snake_case 转换为 PascalCase。
// 例如: "popularity" → "Popularity", "created_at" → "CreatedAt"
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
