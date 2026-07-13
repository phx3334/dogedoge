package initialize

import (
	"fake_tiktok/internal/config"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// InitZap 根据配置初始化 Zap 日志实例
// 支持 JSON 格式输出、日志滚动（基于 lumberjack）和可选的控制台同步输出
// 同时调用 zap.ReplaceGlobals 替换全局 logger
func InitZap(cfg *config.ZapConfig) *zap.Logger {
	// 创建日志写入器（文件 + 滚动）
	writeSyncer := getLogWriter(cfg)

	// 创建 JSON 编码器
	encoder := getEncoder()

	// 创建 zapcore.Core
	core := zapcore.NewCore(encoder, writeSyncer, getLogLevel(cfg.Level))

	var logger *zap.Logger
	if cfg.IsConsolePrint {
		// 同时输出到文件和控制台
		consoleCore := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(
			writeSyncer,
			zapcore.Lock(zapcore.AddSync(os.Stdout)),
		), getLogLevel(cfg.Level))
		logger = zap.New(consoleCore, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	} else {
		// 仅输出到文件
		logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	}

	// 替换全局 logger，使 zap.L() 也能使用
	zap.ReplaceGlobals(logger)
	return logger
}

// getEncoder 创建 JSON 编码器
// 配置时间、级别、调用者等字段格式
func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "@timestamp"
	encoderConfig.LevelKey = "level"
	encoderConfig.NameKey = "logger"
	encoderConfig.CallerKey = "caller"
	encoderConfig.MessageKey = "msg"
	encoderConfig.StacktraceKey = "stacktrace"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	encoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	return zapcore.NewJSONEncoder(encoderConfig)
}

// getLogWriter 创建基于 lumberjack 的日志写入器
// 支持日志文件滚动：按大小切割、按数量保留、按天数清理、自动压缩
func getLogWriter(cfg *config.ZapConfig) zapcore.WriteSyncer {
	lumberJackWriter := &lumberjack.Logger{
		Filename:   cfg.Filename,   // 日志文件路径
		MaxSize:    cfg.MaxSize,    // 单个日志文件最大尺寸（MB）
		MaxBackups: cfg.MaxBackups, // 保留的旧日志文件最大数量
		MaxAge:     cfg.MaxAge,     // 保留旧日志文件的最大天数
		Compress:   cfg.Compress,   // 是否压缩旧日志文件
	}
	return zapcore.AddSync(lumberJackWriter)
}

// getLogLevel 将字符串日志级别转换为 zapcore.Level
func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
