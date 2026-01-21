package logging

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
)

var (
	globalLogger log.Logger
	initOnce     sync.Once
)

// Init 初始化日志系统
func Init(level logrus.Level, enableCaller bool) log.Logger {
	// 创建基础的stream logger
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, level)

	// 如果需要源代码位置，创建包装器
	if enableCaller {
		return &callerLogger{
			Logger: logger,
			level:  level,
		}
	}

	return logger
}

// InitDefault 使用默认配置初始化日志系统
func InitDefault() log.Logger {
	return Init(logrus.InfoLevel, true)
}

// InitDebug 初始化调试级别的日志系统
func InitDebug() log.Logger {
	return Init(logrus.DebugLevel, true)
}

// InitQuiet 初始化安静模式的日志系统（只显示错误）
func InitQuiet() log.Logger {
	return Init(logrus.ErrorLevel, false)
}

// callerLogger 包装器，添加源代码位置信息
type callerLogger struct {
	log.Logger
	level logrus.Level
}

// 获取调用者信息
func (c *callerLogger) getCaller() string {
	// 跳过callerLogger的方法调用链
	// 0: runtime.Callers
	// 1: callerLogger.getCaller
	// 2: callerLogger的日志方法（如Infof）
	// 3: 实际的调用者
	pc := make([]uintptr, 10)
	n := runtime.Callers(3, pc)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pc[:n])
	frame, _ := frames.Next()

	// 简化文件路径，只显示文件名
	filename := path.Base(frame.File)
	// 简化函数名
	funcName := frame.Function
	if lastSlash := strings.LastIndex(funcName, "/"); lastSlash != -1 {
		funcName = funcName[lastSlash+1:]
	}
	if dot := strings.Index(funcName, "."); dot != -1 {
		funcName = funcName[dot+1:]
	}

	return fmt.Sprintf("[%s:%d %s]", filename, frame.Line, funcName)
}

// 重写日志方法以添加调用者信息
func (c *callerLogger) Debugf(format string, args ...interface{}) {
	if c.level >= logrus.DebugLevel {
		caller := c.getCaller()
		c.Logger.Debugf("%s "+format, append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Infof(format string, args ...interface{}) {
	if c.level >= logrus.InfoLevel {
		caller := c.getCaller()
		c.Logger.Infof("%s "+format, append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Warnf(format string, args ...interface{}) {
	if c.level >= logrus.WarnLevel {
		caller := c.getCaller()
		c.Logger.Warnf("%s "+format, append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Errorf(format string, args ...interface{}) {
	if c.level >= logrus.ErrorLevel {
		caller := c.getCaller()
		c.Logger.Errorf("%s "+format, append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Fatalf(format string, args ...interface{}) {
	caller := c.getCaller()
	c.Logger.Fatalf("%s "+format, append([]interface{}{caller}, args...)...)
}

// 其他方法直接委托给底层Logger
func (c *callerLogger) Debug(args ...interface{}) {
	if c.level >= logrus.DebugLevel {
		caller := c.getCaller()
		c.Logger.Debug(append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Info(args ...interface{}) {
	if c.level >= logrus.InfoLevel {
		caller := c.getCaller()
		c.Logger.Info(append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Warn(args ...interface{}) {
	if c.level >= logrus.WarnLevel {
		caller := c.getCaller()
		c.Logger.Warn(append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Error(args ...interface{}) {
	if c.level >= logrus.ErrorLevel {
		caller := c.getCaller()
		c.Logger.Error(append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Fatal(args ...interface{}) {
	caller := c.getCaller()
	c.Logger.Fatal(append([]interface{}{caller}, args...)...)
}

func (c *callerLogger) Done(args ...interface{}) {
	if c.level >= logrus.InfoLevel {
		caller := c.getCaller()
		c.Logger.Done(append([]interface{}{caller}, args...)...)
	}
}

func (c *callerLogger) Donef(format string, args ...interface{}) {
	if c.level >= logrus.InfoLevel {
		caller := c.getCaller()
		c.Logger.Donef("%s "+format, append([]interface{}{caller}, args...)...)
	}
}

// 全局logger函数

// GetGlobalLogger 获取全局logger实例
func GetGlobalLogger() log.Logger {
	initOnce.Do(func() {
		globalLogger = InitDefault()
	})
	return globalLogger
}

// SetGlobalLogger 设置全局logger实例
func SetGlobalLogger(logger log.Logger) {
	globalLogger = logger
}

// 便捷函数

// Debugf 输出调试级别日志
func Debugf(format string, args ...interface{}) {
	GetGlobalLogger().Debugf(format, args...)
}

// Infof 输出信息级别日志
func Infof(format string, args ...interface{}) {
	GetGlobalLogger().Infof(format, args...)
}

// Warnf 输出警告级别日志
func Warnf(format string, args ...interface{}) {
	GetGlobalLogger().Warnf(format, args...)
}

// Errorf 输出错误级别日志
func Errorf(format string, args ...interface{}) {
	GetGlobalLogger().Errorf(format, args...)
}

// Fatalf 输出致命错误级别日志并退出
func Fatalf(format string, args ...interface{}) {
	GetGlobalLogger().Fatalf(format, args...)
}
