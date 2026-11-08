/*
 * @Author: shanghanjin
 * @Date: 2024-12-24 10:20:05
 * @LastEditTime: 2024-12-25 16:15:14
 * @FilePath: \CloudDisk\business\business.go
 * @Description: 业务封装
 */
package business

import (
	"CloudDisk/configwrapper"
	"CloudDisk/dbwrapper"
	"CloudDisk/logwrapper"
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

/**
 * @description: 查询文件夹api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func QueryFolder(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type QueryFolderRequest struct {
		FolderID int64 `json:"folderID"`
	}
	var req QueryFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 查询文件夹信息
	queryResult, err := dbwrapper.QueryFolderInfoFull(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 结果写入响应体
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queryResult)
}

/**
 * @description: 新建文件夹api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func CreateFolder(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type CreateFolderRequest struct {
		FolderName     string `json:"folderName"`
		ParentFolderID int64  `json:"parentFolderID"`
	}
	var req CreateFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 查询父文件夹路径
	parentFolderPath, err := dbwrapper.QueryFolderPath(req.ParentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 拼接路径
	relativePath := path.Join(parentFolderPath, req.FolderName)
	localFullPath := path.Join(GetBaseFolderPath(), relativePath)

	// 检查文件夹是否在本地存在
	_, err = os.Stat(localFullPath)
	if os.IsExist(err) {
		// 检测文件夹是否在数据库中存在
		if exists, err := dbwrapper.FolderExistByPath(relativePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if exists {
			// 如果在数据库中也存在，则返回错误
			http.Error(w, "Folder already exists", http.StatusInternalServerError)
			return
		}

		// 如果在数据库中不存在，则尝试删除本地文件夹
		err = os.RemoveAll(localFullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// 创建本地文件夹
	err = os.MkdirAll(localFullPath, os.ModePerm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 如果后续操作没有正常完成，则删除已创建的本地文件夹
	var operationSuc bool = false
	defer func() {
		if !operationSuc {
			// 删除文件夹及其内容
			removeErr := os.RemoveAll(localFullPath)
			if removeErr != nil {
				// 删除失败时记录日志
				logwrapper.Logger.Errorf("Failed to clean up folder %s: %v", localFullPath, removeErr)
			}
		}
	}()

	// 数据库新建文件夹
	folderID, err := dbwrapper.CreateFolder(req.FolderName, req.ParentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 查询文件夹信息
	folderInfo, err := dbwrapper.QueryFolderInfo(folderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 标记操作成功
	operationSuc = true

	// 结果写入响应体
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folderInfo)
}

/**
 * @description: 上传文件api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func UploadFile(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 获取文件字段
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 获取父文件夹id字段
	parentFolderIDStr := r.FormValue("parentFolderID")
	if parentFolderIDStr == "" {
		http.Error(w, "parentFolderID is required", http.StatusBadRequest)
		return
	}

	parentFolderID, err := strconv.ParseInt(parentFolderIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid parentFolderID", http.StatusBadRequest)
		return
	}

	// 获取文件大小字段
	fileSizeStr := r.FormValue("fileSize")
	if fileSizeStr == "" {
		http.Error(w, "fileSize is required", http.StatusBadRequest)
		return
	}

	fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid fileSize", http.StatusBadRequest)
		return
	}

	// 查询父文件夹路径
	parentFolderPath, err := dbwrapper.QueryFolderPath(parentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 拼接写入路径
	relativePath := path.Join(parentFolderPath, handler.Filename) // path不会自动转换路径分隔符
	localFullPath := path.Join(GetBaseFolderPath(), relativePath) // filepath会自动转换路径分隔符
	MkPathParentFolder(localFullPath)

	// 检查文件是否在本地存在
	if _, err := os.Stat(localFullPath); err == nil {
		// 检测文件是否在数据库中存在
		if exists, err := dbwrapper.FileExistByPath(relativePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if exists {
			// 如果在数据库中也存在，则返回错误
			http.Error(w, "File already exists", http.StatusInternalServerError)
			return
		}

		// 如果在数据库中不存在，则尝试删除本地文件
		err = RemoveFileIgnoreNotExist(localFullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// 标记操作是否成功，如果后续操作没有成功，则删除本地文件
	var operationSuc bool
	defer func() {
		if !operationSuc {
			// 删除文件
			removeErr := RemoveFileIgnoreNotExist(localFullPath)
			if removeErr != nil {
				// 删除失败时记录日志
				logwrapper.Logger.Errorf("Failed to clean up file %s: %v", localFullPath, removeErr)
			}
		}
	}()

	// 创建本地文件
	fileWrite, err := os.Create(localFullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fileWrite.Close()

	// 将上传的文件内容写入本地文件
	if _, err := io.Copy(fileWrite, file); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 写入数据库
	fileID, err := dbwrapper.CreateFile(handler.Filename, fileSize, parentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 查询文件信息
	fileInfo, err := dbwrapper.QueryFileInfo(fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	operationSuc = true

	// 写入响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileInfo)
}

/**
 * @description: 重命名文件夹api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func RenameFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		ID   int64  `json:"folderID"`
		Name string `json:"folderName"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	item, err := dbwrapper.QueryFolderInfo(req.ID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = dbwrapper.Relocate(req.ID, true, item.ParentFolderID, req.Name, GetBaseFolderPath()); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/**
 * @description: 重命名文件api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func RenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		ID   int64  `json:"fileID"`
		Name string `json:"fileName"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	item, err := dbwrapper.QueryFileInfo(req.ID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = dbwrapper.Relocate(req.ID, false, item.ParentFolderID, req.Name, GetBaseFolderPath()); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/**
 * @description: 删除文件夹api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func DeleteFolder(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type DeleteFolderRequest struct {
		FolderID int64 `json:"folderID"`
	}
	var req DeleteFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 根文件夹不允许删除
	if req.FolderID == 1 {
		http.Error(w, "Cannot delete root folder", http.StatusBadRequest)
		return
	}

	// 查询文件夹信息
	folder, err := dbwrapper.QueryFolderInfo(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 删除本地文件夹
	var localPath = path.Join(GetBaseFolderPath(), folder.Path)
	err = os.RemoveAll(localPath) // 路径不存在时，os.RemoveAll也会返回nil
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 删除数据库中的文件夹
	err = dbwrapper.DeleteFolder(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回成功信息
	w.Write([]byte("Folder deleted successfully"))
}

/**
 * @description: 删除文件api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func DeleteFile(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type DeleteFileRequest struct {
		FileID int64 `json:"fileID"`
	}
	var req DeleteFileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 查询文件信息
	fileInfo, err := dbwrapper.QueryFileInfo(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 删除本地文件
	var localPath = path.Join(GetBaseFolderPath(), fileInfo.Path)
	err = RemoveFileIgnoreNotExist(localPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 删除数据库中的文件
	err = dbwrapper.DeleteFile(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回成功信息
	w.Write([]byte("File deleted successfully"))
}

/**
 * @description: 下载文件api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func DownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method is not supported.", http.StatusMethodNotAllowed)
		return
	}

	fileIDStr := r.URL.Query().Get("fileID")
	if fileIDStr == "" {
		http.Error(w, "fileID is required", http.StatusBadRequest)
		return
	}
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid fileID", http.StatusBadRequest)
		return
	}

	fileInfo, err := dbwrapper.QueryFileInfo(fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	w.Header().Set("Content-Disposition", "attachment; filename="+fileInfo.Name)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path.Join(GetBaseFolderPath(), fileInfo.Path))
}

/**
 * @description: 获取基础文件夹的本地路径
 * @return {string} 路径
 */
func GetBaseFolderPath() string {
	// Resolve relative storage paths from the working directory, just like config and logs.
	baseFolderPath, err := filepath.Abs(configwrapper.Cfg.Local.BaseFolder)
	if err != nil {
		logwrapper.Logger.Fatal(err)
	}
	return filepath.ToSlash(baseFolderPath)
}

/**
 * @description: 创建父文件夹
 * @param {string} pathStr
 * @return {*}
 */
func MkPathParentFolder(pathStr string) {
	os.MkdirAll(path.Dir(pathStr), os.ModePerm)
}

/**
 * @description: 以相对路径创建文件夹
 * @param {string} relativePath
 * @return {*}
 */
func MakeAbsoluteFolder(relativePath string) {
	if err := os.MkdirAll(path.Join(GetBaseFolderPath(), relativePath), os.ModePerm); err != nil {
		logwrapper.Logger.Fatal(err)
	}
}

/**
 * @description: 下载文件夹api（打包成ZIP流返回）
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func DownloadFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method is not supported.", http.StatusMethodNotAllowed)
		return
	}

	folderIDStr := r.URL.Query().Get("folderID")
	if folderIDStr == "" {
		http.Error(w, "folderID is required", http.StatusBadRequest)
		return
	}
	folderID, err := strconv.ParseInt(folderIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid folderID", http.StatusBadRequest)
		return
	}

	folderInfo, err := dbwrapper.QueryFolderInfo(folderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	localFolderPath := path.Join(GetBaseFolderPath(), folderInfo.Path)
	basePath := strings.ReplaceAll(path.Dir(localFolderPath), "\\", "/")

	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+folderInfo.Name+".zip\"")
	w.Header().Set("Content-Type", "application/zip")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	err = filepath.Walk(localFolderPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		normalizedFilePath := strings.ReplaceAll(filePath, "\\", "/")
		relPath := strings.TrimPrefix(normalizedFilePath, basePath+"/")

		zipEntry, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(zipEntry, f)
		return err
	})

	if err != nil {
		logwrapper.Logger.Errorf("Failed to create zip for folder %d: %v", folderID, err)
	}
}

/**
 * @description: 删除文件，忽略文件不存在的错误
 * @param {string} path 文件路径
 * @return {*}
 */
func RemoveFileIgnoreNotExist(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // 忽略路径不存在的错误
	}
	return err
}

func MoveItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		ID             int64 `json:"id"`
		IsFolder       bool  `json:"isFolder"`
		TargetFolderID int64 `json:"targetFolderID"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	var name string
	if req.IsFolder {
		v, e := dbwrapper.QueryFolderInfo(req.ID)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		name = v.Name
	} else {
		v, e := dbwrapper.QueryFileInfo(req.ID)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		name = v.Name
	}
	if e := dbwrapper.Relocate(req.ID, req.IsFolder, req.TargetFolderID, name, GetBaseFolderPath()); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func PreviewFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	id, e := strconv.ParseInt(r.URL.Query().Get("fileID"), 10, 64)
	if e != nil {
		http.Error(w, "Invalid fileID", 400)
		return
	}
	f, e := dbwrapper.QueryFileInfo(id)
	if e != nil {
		http.Error(w, e.Error(), 404)
		return
	}
	ext := strings.ToLower(path.Ext(f.Name))
	if ext == ".pdf" {
		w.Header().Set("Content-Type", "application/pdf")
	} else if strings.Contains("|.txt|.md|.markdown|.json|.log|.csv|.tsv|.xml|.yaml|.yml|.ini|.conf|.toml|.go|.js|.ts|.jsx|.tsx|.vue|.html|.css|.scss|.py|.java|.c|.cpp|.h|.sh|.sql|.dart|.rs|.bat|.ps1|", "|"+ext+"|") && ext != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		http.Error(w, "暂不支持预览此类型文件", 415)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path.Join(GetBaseFolderPath(), f.Path))
}

var sessions = struct {
	sync.Mutex
	values map[string]time.Time
}{values: make(map[string]time.Time)}

func isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie("homepage_session")
	if err != nil {
		return false
	}
	sessions.Lock()
	defer sessions.Unlock()
	expiry, ok := sessions.values[cookie.Value]
	return ok && time.Now().Before(expiry)
}

// Permissions wrap all API routes, including the legacy disk mutation endpoints.
func Permissions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			read := r.Method == http.MethodGet || r.Method == http.MethodHead || (r.URL.Path == "/api/queryFolder" && r.Method == http.MethodPost)
			auth := r.URL.Path == "/api/login"
			if !read {
				if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
					http.Error(w, "来源不受信任", 403)
					return
				}
				if !auth && !isAdmin(r) {
					http.Error(w, "请先以管理员身份登录", 401)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

var loginLimit = struct {
	sync.Mutex
	attempts int
	until    time.Time
}{}

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	loginLimit.Lock()
	defer loginLimit.Unlock()
	if time.Now().After(loginLimit.until) {
		loginLimit.attempts = 0
		loginLimit.until = time.Now().Add(time.Minute)
	}
	if loginLimit.attempts >= 10 {
		http.Error(w, "尝试过于频繁，请稍后重试", 429)
		return
	}
	loginLimit.attempts++
	var req struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	configured := configwrapper.Cfg.AdminPasswordHash
	if configured == "" {
		http.Error(w, "请在服务端 config 中配置 adminPasswordHash（密码的 SHA-256）", 503)
		return
	}
	hash := sha256.Sum256([]byte(req.Password))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(hash[:])), []byte(configured)) != 1 {
		http.Error(w, "密码错误", 401)
		return
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		http.Error(w, "登录失败", 500)
		return
	}
	sessions.Lock()
	for k, v := range sessions.values {
		if time.Now().After(v) {
			delete(sessions.values, k)
		}
	}
	sessions.values[hex.EncodeToString(token)] = time.Now().Add(12 * time.Hour)
	sessions.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "homepage_session", Value: hex.EncodeToString(token), Path: "/api", HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	w.WriteHeader(204)
}
func Session(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		if c, e := r.Cookie("homepage_session"); e == nil {
			sessions.Lock()
			delete(sessions.values, c.Value)
			sessions.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "homepage_session", Path: "/api", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"admin": isAdmin(r)})
}
func Homepage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		body, version, err := dbwrapper.ReadHomepage()
		if err != nil {
			http.Error(w, "读取失败", 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": json.RawMessage(body), "version": version})
	case http.MethodPut:
		var req struct {
			Data    json.RawMessage `json:"data"`
			Version int64           `json:"version"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&req) != nil || !json.Valid(req.Data) {
			http.Error(w, "内容无效或超过 8 MB", 400)
			return
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(req.Data, &object) != nil || object == nil {
			http.Error(w, "内容格式无效", 400)
			return
		}
		if err := dbwrapper.SaveHomepage(string(req.Data), req.Version); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		json.NewEncoder(w).Encode(map[string]int64{"version": req.Version + 1})
	default:
		http.Error(w, "Method not allowed", 405)
	}
}
