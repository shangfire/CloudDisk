/*
 * @Author: shanghanjin
 * @Date: 2024-08-12 11:38:02
 * @LastEditTime: 2024-09-24 19:47:19
 * @FilePath: \CloudDisk\main.go
 * @Description:main
 */
package main

import (
	"CloudDisk/business"
	"CloudDisk/configwrapper"
	"CloudDisk/dbwrapper"
	"CloudDisk/logwrapper"
	"net/http"

	"github.com/sirupsen/logrus"
)

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

	// 初始化基础文件夹
	business.MakeAbsoluteFolder("")

	// 提供浏览页面的服务
	queryFS := http.FileServer(http.Dir("./html/query"))
	http.Handle("/query/", http.StripPrefix("/query", queryFS))

	// 设置各接口响应函数
	http.HandleFunc("/api/queryFolder", business.QueryFolder)
	http.HandleFunc("/api/uploadFile", business.UploadFile)
	http.HandleFunc("/api/createFolder", business.CreateFolder)
	http.HandleFunc("/api/deleteFile", business.DeleteFile)
	http.HandleFunc("/api/deleteFolder", business.DeleteFolder)

	logwrapper.Logger.Info("Server is running")

	// 启动服务
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
