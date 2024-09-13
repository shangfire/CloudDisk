/*
 * @Author: shanghanjin
 * @Date: 2024-08-12 11:38:01
 * @LastEditTime: 2024-08-30 15:46:23
 * @FilePath: \UserFeedBack\logwrapper\log.go
 * @Description: 日志封装
 */
package logwrapper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// 日志库导出单例
var Logger *logrus.Logger

// 自定义空结构体
type CustomFormatter struct{}

/**
 * @description: 实现logrus.Formatter接口，将日志按照期望的格式输出
 * @param {*logrus.Entry} entry 日志相关参数结构体
 * @return {*} format之后的日志信息
 */
func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// 构建时间戳
	timestamp := entry.Time.Format(time.RFC3339)

	// 获取文件和行号
	var fileLine string
	if entry.HasCaller() {
		frame := entry.Caller
		funcName := filepath.Base(frame.Function)
		fileName := filepath.Base(frame.File)
		fileLine = fmt.Sprintf("[%s:%d][%s]", funcName, frame.Line, fileName)
	}

	// 构建日志消息
	msg := fmt.Sprintf("[%s]%s%s\n", timestamp, entry.Message, fileLine)

	// 返回日志条目
	return []byte(msg), nil
}

/**
 * @description: 初始化函数，设置日志输出路径和级别
 * @param {string} logPath 日志输出路径
 * @param {logrus.Level} logLevel 日志级别
 * @return {*}
 */
func Init(logPath string, logLevel logrus.Level) error {
	// 日志目录不存在且创建失败则返回错误
	dir := filepath.Dir(logPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// 初始化日志参数
	logWriter := &lumberjack.Logger{
		Filename:   logPath, // 日志文件路径
		MaxSize:    100,     // 每个日志文件的最大尺寸 单位：M
		MaxBackups: 3,       // 最多保留的日志文件个数
		MaxAge:     28,      // 保留旧文件的最大天数
	}

	// 创建logrus
	Logger = logrus.New()

	// 设置同时输出到控制台和文件
	Logger.SetOutput(io.MultiWriter(os.Stdout, logWriter))

	// 设置日志记录调用者信息，从而能够获得function等信息
	Logger.SetReportCaller(true)

	// 设置formatter
	Logger.SetFormatter(&CustomFormatter{})

	// 设置日志级别
	Logger.SetLevel(logLevel)

	return nil
}
