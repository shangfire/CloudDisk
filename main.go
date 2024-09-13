/*
 * @Author: shanghanjin
 * @Date: 2024-08-12 11:38:02
 * @LastEditTime: 2024-09-09 13:58:23
 * @FilePath: \UserFeedBack\main.go
 * @Description:main
 */
package main

import (
	"CloudDisk/configwrapper"
	"CloudDisk/dbwrapper"
	"CloudDisk/logwrapper"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
)

/**
 * @description: 汇报反馈接口
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func queryFolder(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 响应客户端已完成
	fmt.Fprintf(w, "Files uploaded successfully")
}

func main() {
	// 初始化日志库
	if err := logwrapper.Init("./log/log.log", logrus.DebugLevel); err != nil {
		logwrapper.Logger.Fatal(err)
	}

	// 初始化配置
	if err := configwrapper.Init("./config"); err != nil {
		logwrapper.Logger.Fatal(err)
	}

	// 初始化数据库
	dbwrapper.InitDB()
	defer dbwrapper.CloseDB()

	// 提供浏览页面的服务
	queryFS := http.FileServer(http.Dir("./html/query"))
	http.Handle("/query/", http.StripPrefix("/query", queryFS))

	// 设置各接口响应函数
	http.HandleFunc("/api/queryFolder", queryFolder)

	logwrapper.Logger.Info("Server is running")

	// 启动服务
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
