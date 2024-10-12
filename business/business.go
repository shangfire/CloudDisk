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

func QueryFolder(w http.ResponseWriter, r *http.Request) {
	type QueryFolderRequest struct {
		FolderID *int64 `json:"folderID,omitempty"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req QueryFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// ensure FolderID to be nil when it is 0
	if req.FolderID != nil && *req.FolderID == 0 {
		req.FolderID = nil
	}

	// query file
	queryResult, err := dbwrapper.QueryFolderContent(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// convert queryResult to json and write to response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queryResult)
}

func UploadFile(w http.ResponseWriter, r *http.Request) {
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
	localFullPath := path.Join(GetBaseFolderPath(), parentFolderPath, handler.Filename) // filepath会自动转换路径分隔符
	MkPathFolder(localFullPath)
	relativePath := path.Join(parentFolderPath, handler.Filename) // path不会自动转换路径分隔符

	// 复制文件内容
	fileWrite, err := os.Create(localFullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fileWrite.Close()

	if _, err := io.Copy(fileWrite, file); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 写入数据库
	_, err = dbwrapper.CreateFile(handler.Filename, relativePath, fileSize, parentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("File uploaded successfully"))
}

func CreateFolder(w http.ResponseWriter, r *http.Request) {
	type CreateFolderRequest struct {
		FolderName     string `json:"folderName"`
		FolderPath     string `json:"folderPath"`
		ParentFolderID *int64 `json:"parentFolderID,omitempty"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req CreateFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// create folder of file path
	var fullPath = configwrapper.Cfg.Local.BaseFolder + req.FolderPath
	err = os.MkdirAll(fullPath+req.FolderName, os.ModePerm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = dbwrapper.CreateFolder(req.FolderName, req.FolderPath, req.ParentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Folder created successfully"))
}

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	type DeleteFileRequest struct {
		FileName string `json:"fileName"`
		FilePath string `json:"filePath"`
		FileID   int64  `json:"fileID"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req DeleteFileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// delete file from local storage
	var fullPath = configwrapper.Cfg.Local.BaseFolder + req.FilePath + req.FileName
	err = os.Remove(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = dbwrapper.DeleteFile(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("File deleted successfully"))
}

func DeleteFolder(w http.ResponseWriter, r *http.Request) {
	type DeleteFolderRequest struct {
		FolderName string `json:"folderName"`
		FolderPath string `json:"folderPath"`
		FolderID   int64  `json:"folderID"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req DeleteFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// delete folder from local storage
	var fullPath = configwrapper.Cfg.Local.BaseFolder + req.FolderPath + req.FolderName
	err = os.RemoveAll(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = dbwrapper.DeleteFolder(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Folder deleted successfully"))
}

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

func MkPathFolder(pathStr string) {
	os.MkdirAll(path.Dir(pathStr), os.ModePerm)
}

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
