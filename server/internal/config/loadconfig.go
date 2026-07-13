package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Host           string   `mapstructure:"host"`            // 服务监听主机地址
	Port           int      `mapstructure:"port"`            // 服务监听端口
	SessionsSecret string   `mapstructure:"sessions_secret"` // Session 加密密钥
	UseMultipoint  bool     `mapstructure:"use_multipoint"`  // 是否启用多点登录限制
	RouterPrefix   string   `mapstructure:"router_prefix"`   // 路由前缀
	AllowedOrigins []string `mapstructure:"allowed_origins"` // WebSocket 允许的 Origin 白名单，["*"] 表示允许所有来源
}

// UploadConfig 上传配置
type UploadConfig struct {
	Path          string `mapstructure:"path"`            // 上传文件存储路径
	MaxFileSize   int64  `mapstructure:"max_file_size"`   // 最大文件大小（字节）
	TempUploadDir string `mapstructure:"temp_upload_dir"` // 视频草稿临时上传目录，原始文件先原子写入此目录等待转码
}

// TranscodeConfig 转码任务配置
type TranscodeConfig struct {
	QueueName   string `mapstructure:"queue_name"`  // RabbitMQ 转码队列名，worker 消费该队列触发 ffmpeg 转码
	Concurrency int    `mapstructure:"concurrency"` // 转码并发数，控制同时执行的 ffmpeg 进程数
	Timeout     int    `mapstructure:"timeout"`     // 单次转码超时时间（秒），超时后杀死 ffmpeg 进程并标记失败
}

// StorageConfig 存储抽象配置
// 控制视频/封面/头像等媒体文件落到 LocalStorage 还是 QiniuStorage，按 driver 自动选择实现
type StorageConfig struct {
	Driver  string `mapstructure:"driver"`   // 存储驱动：local 走本地磁盘，qiniu 走七牛云对象存储
	BaseDir string `mapstructure:"base_dir"` // LocalStorage 的本地磁盘根目录（如 ./uploads）
}

// QiniuConfig 七牛云对象存储配置
// 当 storage.driver=qiniu 时生效，视频/封面/头像/通用图片均上传至七牛 Bucket
type QiniuConfig struct {
	Zone          string `mapstructure:"zone"`            // 存储区域：z0(华东) z1(华北) z2(华南) na0(北美) as0(东南亚)
	Bucket        string `mapstructure:"bucket"`          // 存储空间名称
	Domain        string `mapstructure:"domain"`          // 绑定的公开访问域名（用于拼接文件访问 URL）
	AccessKey     string `mapstructure:"access_key"`      // 七牛 AccessKey
	SecretKey     string `mapstructure:"secret_key"`      // 七牛 SecretKey
	UseHTTPS      bool   `mapstructure:"use_https"`       // 访问 URL 是否使用 https
	UseCdnDomains bool   `mapstructure:"use_cdn_domains"` // 上传是否走 CDN 加速域名
	MaxFileSize   int64  `mapstructure:"max_file_size"`   // 单文件最大字节数
}

// FFmpegConfig ffmpeg 转码参数配置
type FFmpegConfig struct {
	BinaryPath  string `mapstructure:"binary_path"`  // ffmpeg 可执行文件路径，容器内默认 ffmpeg（在 PATH 中）
	OutputCodec string `mapstructure:"output_codec"` // 输出视频编码器，如 libx264
	CRF         int    `mapstructure:"crf"`          // 恒定码率因子（18~28，越小质量越好、文件越大）
	Preset      string `mapstructure:"preset"`       // 编码预设（ultrafast/fast/medium/slow 等）
}

// EmailConfig 邮件配置
type EmailConfig struct {
	Host     string `mapstructure:"host"`     // SMTP 服务器地址
	Port     int    `mapstructure:"port"`     // SMTP 服务器端口
	From     string `mapstructure:"from"`     // 发件人邮箱
	Nickname string `mapstructure:"nickname"` // 发件人昵称
	Secret   string `mapstructure:"secret"`   // SMTP 授权码
	IsTLS    bool   `mapstructure:"is_tls"`   // 是否使用 TLS
}

// JWTConfig JWT 配置
type JWTConfig struct {
	AccessTokenSecret      string `mapstructure:"access_token_secret"`       // AccessToken 签名密钥
	RefreshTokenSecret     string `mapstructure:"refresh_token_secret"`      // RefreshToken 签名密钥
	AccessTokenExpiryTime  string `mapstructure:"access_token_expiry_time"`  // AccessToken 过期时间
	RefreshTokenExpiryTime string `mapstructure:"refresh_token_expiry_time"` // RefreshToken 过期时间
	Issuer                 string `mapstructure:"issuer"`                    // Token 签发者
}

// CaptchaConfig 验证码配置
type CaptchaConfig struct {
	Height   int     `mapstructure:"height"`    // 验证码图片高度
	Width    int     `mapstructure:"width"`     // 验证码图片宽度
	Length   int     `mapstructure:"length"`    // 验证码字符长度
	MaxSkew  float64 `mapstructure:"max_skew"`  // 最大倾斜度
	DotCount int     `mapstructure:"dot_count"` // 干扰点数量
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string `mapstructure:"host"`              // 数据库主机地址
	Port            int    `mapstructure:"port"`              // 数据库端口
	User            string `mapstructure:"user"`              // 数据库用户名
	Password        string `mapstructure:"password"`          // 数据库密码
	DBName          string `mapstructure:"dbname"`            // 数据库名称
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // 最大打开连接数
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // 连接最大存活时间（秒），超过后连接会被关闭重建
	// 行业标准：MySQL wait_timeout 通常为 8h(28800s)，建议设为 5~30 分钟
	// 防止连接池中使用已被服务端关闭的"过期连接"导致查询报错
	ConnMaxIdleTime int `mapstructure:"conn_max_idle_time"` // 连接最大空闲时间（秒），超过后空闲连接会被关闭
	// 行业标准：建议设为 ConnMaxLifetime 的 1/2 ~ 1/3
	// 避免空闲连接长期占用资源，同时减少新建连接的开销
	AutoMigrate bool `mapstructure:"auto_migrate"` // 是否启动时自动迁移
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host      string `mapstructure:"host"`       // Redis 主机地址
	Port      int    `mapstructure:"port"`       // Redis 端口
	Password  string `mapstructure:"password"`   // Redis 密码
	DB        int    `mapstructure:"db"`         // Redis 数据库编号
	KeyPrefix string `mapstructure:"key_prefix"` // Redis 键前缀
}

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	Host     string `mapstructure:"host"`     // RabbitMQ 主机地址
	Port     int    `mapstructure:"port"`     // RabbitMQ 端口
	Username string `mapstructure:"username"` // RabbitMQ 用户名
	Password string `mapstructure:"password"` // RabbitMQ 密码
}

// ElasticsearchConfig Elasticsearch 配置
type ElasticsearchConfig struct {
	Host     string `mapstructure:"host"`     // ES 主机地址
	Username string `mapstructure:"username"` // ES 用户名
	Password string `mapstructure:"password"` // ES 密码
}

// GaodeConfig 高德地图配置
type GaodeConfig struct {
	APIKey string `mapstructure:"api_key"` // 高德地图 API Key
}

// ZapConfig Zap 日志配置
type ZapConfig struct {
	Level          string `mapstructure:"level"`            // 日志级别（debug/info/warn/error）
	Filename       string `mapstructure:"filename"`         // 日志文件路径
	MaxSize        int    `mapstructure:"max_size"`         // 单个日志文件最大尺寸（MB）
	MaxBackups     int    `mapstructure:"max_backups"`      // 保留的旧日志文件最大数量
	MaxAge         int    `mapstructure:"max_age"`          // 保留旧日志文件的最大天数
	Compress       bool   `mapstructure:"compress"`         // 是否压缩旧日志文件
	IsConsolePrint bool   `mapstructure:"is_console_print"` // 是否同时输出到控制台
}

// AIConfig AI 聊天配置（OpenAI 兼容接口，如 DeepSeek）
type AIConfig struct {
	APIKey  string `mapstructure:"api_key"`  // API Key，通过环境变量 APP_AI_API_KEY 配置
	BaseURL string `mapstructure:"base_url"` // API 基地址，如 https://api.deepseek.com/v1
	Model   string `mapstructure:"model"`    // 模型名称，如 deepseek-chat
}

// Config 应用总配置
// 聚合所有子配置模块
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`        // 服务器配置
	Upload        UploadConfig        `mapstructure:"upload"`        // 上传配置
	Email         EmailConfig         `mapstructure:"email"`         // 邮件配置
	JWT           JWTConfig           `mapstructure:"jwt"`           // JWT 配置
	Captcha       CaptchaConfig       `mapstructure:"captcha"`       // 验证码配置
	Database      DatabaseConfig      `mapstructure:"database"`      // 数据库配置
	Redis         RedisConfig         `mapstructure:"redis"`         // Redis 配置
	RabbitMQ      RabbitMQConfig      `mapstructure:"rabbitmq"`      // RabbitMQ 配置
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"` // Elasticsearch 配置
	Gaode         GaodeConfig         `mapstructure:"gaode"`         // 高德地图配置
	Zap           ZapConfig           `mapstructure:"zap"`           // Zap 日志配置
	Transcode     TranscodeConfig     `mapstructure:"transcode"`     // 转码任务配置（队列名/并发数/超时）
	Storage       StorageConfig       `mapstructure:"storage"`       // 存储抽象配置（local/qiniu）
	Qiniu         QiniuConfig         `mapstructure:"qiniu"`         // 七牛云对象存储配置
	FFmpeg        FFmpegConfig        `mapstructure:"ffmpeg"`        // ffmpeg 转码参数配置
	AI            AIConfig            `mapstructure:"ai"`            // AI 聊天配置（DeepSeek）

	// RunCron 控制本进程是否注册并启动 cron 任务（Task 5）。
	//   - 平铺在 Config 根，避免 "APP_APP_RUN_CRON" 这种重复前缀
	//   - YAML key：`run_cron`，环境变量：`APP_RUN_CRON`
	//   - API 进程保持 false（http 服务 + 业务写路径职责）
	//   - worker 进程通过 `APP_RUN_CRON=true` 覆盖，独自承接定时任务
	RunCron bool `mapstructure:"run_cron"`
}

// LoadConfig 加载应用配置
// 通过 Viper 从 YAML 配置文件、.env 文件和环境变量中读取配置
// 支持通过 CONFIG_NAME 环境变量选择不同的配置文件
func LoadConfig() (*Config, error) {
	// 1. 优先加载 .env 文件（不覆盖已存在的环境变量）
	//    这样 .env 中的值会被 viper.AutomaticEnv 读取到，但系统环境变量优先级更高
	if err := godotenv.Load(".env"); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("加载 .env 文件失败: %v", err)
		}
	} else {
		log.Println("成功加载 .env 文件")
	}

	cfg := &Config{}
	v := viper.New()

	// 通过环境变量选择配置文件
	configName := os.Getenv("CONFIG_NAME")
	if configName == "" {
		configName = v.GetString("CONFIG_NAME")
	}
	if configName == "" {
		configName = "config.docker" // 默认使用容器/统一配置
	}

	log.Printf("正在加载配置文件: %s.yaml", configName)

	// 设置 Viper 配置
	v.SetConfigType("yaml")

	// 设置环境变量支持
	v.SetEnvPrefix("APP")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取配置文件内容并展开其中的 ${VAR} / ${VAR:-default} 环境变量占位符，
	// 使配置项统一由环境变量驱动（缺省时回退到 :- 后的默认值），
	// 然后再交由 viper 解析 YAML。
	configPath, found := findConfigFile(configName)
	if !found {
		log.Printf("配置文件未找到: %s.yaml，使用环境变量配置", configName)
	} else {
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		expanded := expandEnvPlaceholders(string(raw))
		if err := v.ReadConfig(bytes.NewReader([]byte(expanded))); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
		log.Printf("成功加载配置文件: %s", configPath)
	}

	// 将配置绑定到结构体
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 验证必要配置
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return cfg, nil
}

// findConfigFile 在候选目录中查找 {configName}.yaml，返回首个存在的绝对路径。
func findConfigFile(configName string) (string, bool) {
	candidates := []string{
		"./configs/" + configName + ".yaml",
		"/app/configs/" + configName + ".yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// expandEnvPlaceholders 展开字符串中的 ${VAR} 与 ${VAR:-default} 占位符。
//   - ${VAR}            → 环境变量 VAR 的值（未设置则为空）
//   - ${VAR:-default}   → 环境变量 VAR 的值，未设置或为空时回退到 default
//
// 使用 os.Expand 逐个匹配 ${...}，仅支持大括号形式，避免误伤 YAML 中的普通 $ 字符。
func expandEnvPlaceholders(s string) string {
	return os.Expand(s, func(key string) string {
		if idx := strings.Index(key, ":-"); idx >= 0 {
			name := key[:idx]
			def := key[idx+2:]
			if val := os.Getenv(name); val != "" {
				return val
			}
			return def
		}
		return os.Getenv(key)
	})
}

// validateConfig 验证配置的必要字段
func validateConfig(cfg *Config) error {
	if cfg.Server.Port <= 0 {
		return fmt.Errorf("服务器端口必须大于0")
	}
	if cfg.Database.Host == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if cfg.Redis.Host == "" {
		return fmt.Errorf("Redis主机不能为空")
	}
	if cfg.RabbitMQ.Host == "" {
		return fmt.Errorf("RabbitMQ主机不能为空")
	}
	if cfg.Elasticsearch.Host == "" {
		return fmt.Errorf("Elasticsearch主机配置不能为空")
	}
	// 存储驱动校验：必须显式声明 local 或 qiniu
	// 默认未配置时拒绝启动，避免上传链路默默落到错误实现
	if cfg.Storage.Driver != "local" && cfg.Storage.Driver != "qiniu" {
		return fmt.Errorf("storage.driver 必须为 local 或 qiniu，当前为 %q", cfg.Storage.Driver)
	}
	// 选择 qiniu 时强制要求访问凭证与空间信息，缺失会直接导致上传失败
	if cfg.Storage.Driver == "qiniu" {
		if cfg.Qiniu.Bucket == "" {
			return fmt.Errorf("storage.driver=qiniu 时 qiniu.bucket 不能为空")
		}
		if cfg.Qiniu.Domain == "" {
			return fmt.Errorf("storage.driver=qiniu 时 qiniu.domain 不能为空")
		}
		if cfg.Qiniu.AccessKey == "" {
			return fmt.Errorf("storage.driver=qiniu 时 qiniu.access_key 不能为空")
		}
		if cfg.Qiniu.SecretKey == "" {
			return fmt.Errorf("storage.driver=qiniu 时 qiniu.secret_key 不能为空")
		}
	}
	return nil
}
