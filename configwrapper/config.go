/*
 * @Author: shanghanjin
 * @Date: 2024-08-19 17:51:57
 * @LastEditTime: 2024-08-30 15:58:17
 * @FilePath: \UserFeedBack\configwrapper\config.go
 * @Description: 配置封装
 */
package configwrapper

import (
	logger "CloudDisk/logwrapper"
	"encoding/json"
	"os"
)

type Local struct {
	BaseFolder string `json:"baseFolder"`
}

type Database struct {
	User     string `json:"user"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Schema   string `json:"schema"`
	Password string `json:"password"`
}

type Config struct {
	Local    Local    `json:"local"`
	Database Database `json:"database"`
}

var Cfg *Config

/**
 * @description: 读取初始化文件
 * @param {string} configFilePath config文件路径
 * @return {*}
 */
func Init(configFilePath string) error {
	Cfg = &Config{}

	// 读取文件
	configData, err := os.ReadFile(configFilePath)
	if err != nil {
		logger.Logger.Fatalf("Error reading config file: %v", err)
	}

	// 反序列化
	err = json.Unmarshal(configData, Cfg)
	if err != nil {
		logger.Logger.Fatalf("Error parsing config file: %v", err)
	}

	return nil
}
