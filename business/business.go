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
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
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
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type RenameFolderRequest struct {
		FolderName string `json:"folderName"`
		FolderID   int64  `json:"folderID"`
	}
	var req RenameFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// root文件夹无法重命名
	if req.FolderID == 1 {
		http.Error(w, "Cannot rename root folder", http.StatusBadRequest)
		return
	}

	// 查询父文件夹路径
	parentFolderPath, err := dbwrapper.QueryFolderPath(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 重命名文件夹
	var oldPath = path.Join(GetBaseFolderPath(), parentFolderPath)
	var newPath = path.Join(path.Dir(oldPath), req.FolderName)
	err = os.Rename(oldPath, newPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 如果后续操作没有正常完成，则回滚重命名操作
	var operationSuc bool
	defer func() {
		if !operationSuc {
			// 回滚重命名操作
			err := os.Rename(newPath, oldPath)
			if err != nil {
				logwrapper.Logger.Errorf("Failed to rollback folder rename: %v", err)
			}
		}
	}()

	// 更新数据库中的文件夹名称
	err = dbwrapper.RenameFolder(req.FolderID, req.FolderName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	operationSuc = true

	// 返回成功信息
	w.Write([]byte("Folder renamed successfully"))
}

/**
 * @description: 重命名文件api
 * @param {http.ResponseWriter} w
 * @param {*http.Request} r
 * @return {*}
 */
func RenameFile(w http.ResponseWriter, r *http.Request) {
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type RenameFileRequest struct {
		FileName string `json:"fileName"`
		FileID   int64  `json:"fileID"`
	}
	var req RenameFileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 查询文件信息
	file, err := dbwrapper.QueryFileInfo(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 重命名文件
	var oldPath = path.Join(GetBaseFolderPath(), file.Path)
	var newPath = path.Join(path.Dir(oldPath), req.FileName)
	err = os.Rename(oldPath, newPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 如果后续操作没有正常完成，则回滚重命名操作
	var operationSuc bool
	defer func() {
		if !operationSuc {
			// 回滚重命名操作
			err := os.Rename(newPath, oldPath)
			if err != nil {
				logwrapper.Logger.Errorf("Failed to rollback file rename: %v", err)
			}
		}
	}()

	// 更新数据库中的文件名称
	err = dbwrapper.RenameFile(req.FileID, req.FileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	operationSuc = true

	// 返回成功信息
	w.Write([]byte("File renamed successfully"))
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
	// 只支持POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// 解析请求体
	type DownloadFileRequest struct {
		FileID int64 `json:"fileID"`
	}
	var req DownloadFileRequest
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

	// 提供下载文件响应
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
	exePath, err := os.Executable()
	if err != nil {
		logwrapper.Logger.Fatal(err)
	}

	exePath = strings.ReplaceAll(exePath, "\\", "/")
	exeFolder := path.Dir(exePath)
	baseFolderPath := path.Join(exeFolder, configwrapper.Cfg.Local.BaseFolder)
	return baseFolderPath
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
	exePath, err := os.Executable()
	if err != nil {
		logwrapper.Logger.Fatal(err)
	}

	exePath = strings.ReplaceAll(exePath, "\\", "/")
	exeFolder := path.Dir(exePath)
	absoluteFolder := path.Join(exeFolder, configwrapper.Cfg.Local.BaseFolder, relativePath)
	os.MkdirAll(absoluteFolder, os.ModePerm)
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
