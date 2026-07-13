package es

import (
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

// LogIndex 日志 ES 索引
func LogIndex() string {
	return "logs"
}

// LogMapping 定义 Elasticsearch 中日志索引的映射结构
// 作用：告诉 Elasticsearch 如何索引和存储日志的各个字段，优化搜索和分析性能
// 返回值：
//
//	*types.TypeMapping: 日志索引的映射定义
func LogMapping() *types.TypeMapping {
	return &types.TypeMapping{
		Properties: map[string]types.Property{
			// 时间戳，日期类型
			"@timestamp": types.DateProperty{
				Format: &[]string{"strict_date_optional_time||epoch_millis"}[0],
			},
			// 日志级别，关键字类型，用于精确匹配
			"level": types.KeywordProperty{},
			// 日志消息，文本类型，支持全文搜索
			"message": types.TextProperty{
				Analyzer:       &[]string{"standard"}[0],
				SearchAnalyzer: &[]string{"standard"}[0],
			},
			// 服务名称，关键字类型，用于精确匹配
			"service": types.KeywordProperty{},
			// HTTP 方法，关键字类型，用于精确匹配
			"method": types.KeywordProperty{},
			// 请求路径，关键字类型，用于精确匹配
			"path": types.KeywordProperty{},
			// HTTP 状态码，整数类型，用于统计
			"status": types.IntegerNumberProperty{},
			// 客户端 IP，IP 类型
			"ip": types.IpProperty{},
			// 用户代理，关键字类型
			"user_agent": types.KeywordProperty{},
			// 请求耗时，浮点数类型，用于统计
			"cost": types.FloatNumberProperty{},
			// 错误消息，文本类型，支持全文搜索
			"error_message": types.TextProperty{
				Analyzer:       &[]string{"standard"}[0],
				SearchAnalyzer: &[]string{"standard"}[0],
			},
			// 堆栈跟踪，文本类型，支持全文搜索
			"stack_trace": types.TextProperty{
				Analyzer:       &[]string{"standard"}[0],
				SearchAnalyzer: &[]string{"standard"}[0],
			},
			// 请求 ID，关键字类型，用于精确匹配
			"request_id": types.KeywordProperty{},
			// 用户 ID，关键字类型，用于精确匹配
			"user_id": types.KeywordProperty{},
		},
	}
}
