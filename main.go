/*
 * @Author: shanghanjin
 * @Date: 2024-08-12 11:38:02
 * @LastEditTime: 2024-12-22 17:22:36
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

	"github.com/rs/cors"
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

	// 创建一个新的多路复用器
	mux := http.NewServeMux()

	// 提供浏览页面的服务
	queryFS := http.FileServer(http.Dir("./html/query"))
	mux.Handle("/query/", http.StripPrefix("/query", queryFS))

	// 设置各接口响应函数
	mux.HandleFunc("/api/queryFolder", business.QueryFolder)
	mux.HandleFunc("/api/createFolder", business.CreateFolder)
	mux.HandleFunc("/api/uploadFile", business.UploadFile)
	mux.HandleFunc("/api/renameFolder", business.RenameFolder)
	mux.HandleFunc("/api/renameFile", business.RenameFile)
	mux.HandleFunc("/api/deleteFile", business.DeleteFile)
	mux.HandleFunc("/api/deleteFolder", business.DeleteFolder)
	mux.HandleFunc("/api/downloadFile", business.DownloadFile)

	// 设置跨域请求
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Content-Disposition"},
	})

	handler := c.Handler(mux)

	logwrapper.Logger.Info("Server is running")

	// 启动服务
	if err := http.ListenAndServe(":8080", handler); err != nil {
		panic(err)
	}
}
