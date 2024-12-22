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
	// enableCORS(w, r)

	type QueryFolderRequest struct {
		FolderID int64 `json:"folderID"`
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

func CreateFolder(w http.ResponseWriter, r *http.Request) {
	type CreateFolderRequest struct {
		FolderName     string `json:"folderName"`
		ParentFolderID int64  `json:"parentFolderID"`
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

	parentFolderPath, err := dbwrapper.QueryFolderPath(req.ParentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// create folder of file path
	relativePath := path.Join(parentFolderPath, req.FolderName)
	localFullPath := path.Join(GetBaseFolderPath(), relativePath)
	// failed if folder already exists
	_, err = os.Stat(localFullPath)
	if os.IsExist(err) {
		http.Error(w, "Folder already exists", http.StatusBadRequest)
		return
	}

	err = os.MkdirAll(localFullPath, os.ModePerm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folderInfo)
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
	relativePath := path.Join(parentFolderPath, handler.Filename) // path不会自动转换路径分隔符
	localFullPath := path.Join(GetBaseFolderPath(), relativePath) // filepath会自动转换路径分隔符
	MkPathFolder(localFullPath)

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
	fileID, err := dbwrapper.CreateFile(handler.Filename, fileSize, parentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileInfo, err := dbwrapper.QueryFileInfo(fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileInfo)
}

func RenameFolder(w http.ResponseWriter, r *http.Request) {
	type RenameFolderRequest struct {
		FolderName string `json:"folderName"`
		FolderID   int64  `json:"folderID"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req RenameFolderRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.FolderID == 1 {
		http.Error(w, "Cannot rename root folder", http.StatusBadRequest)
		return
	}

	parentFolderPath, err := dbwrapper.QueryFolderPath(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var oldPath = path.Join(GetBaseFolderPath(), parentFolderPath)
	var newPath = path.Join(path.Dir(oldPath), req.FolderName)
	err = os.Rename(oldPath, newPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = dbwrapper.RenameFolder(req.FolderID, req.FolderName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Folder renamed successfully"))
}

func RenameFile(w http.ResponseWriter, r *http.Request) {
	type RenameFileRequest struct {
		FileName string `json:"fileName"`
		FileID   int64  `json:"fileID"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req RenameFileRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, err := dbwrapper.QueryFileInfo(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var oldPath = path.Join(GetBaseFolderPath(), file.Path)
	var newPath = path.Join(path.Dir(oldPath), req.FileName)
	err = os.Rename(oldPath, newPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = dbwrapper.RenameFile(req.FileID, req.FileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("File renamed successfully"))
}

func DeleteFolder(w http.ResponseWriter, r *http.Request) {
	type DeleteFolderRequest struct {
		FolderID int64 `json:"folderID"`
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

	if req.FolderID == 1 {
		http.Error(w, "Cannot delete root folder", http.StatusBadRequest)
		return
	}

	folder, err := dbwrapper.QueryFolderInfo(req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var localPath = path.Join(GetBaseFolderPath(), folder.Path)
	err = os.RemoveAll(localPath)
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

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	type DeleteFileRequest struct {
		FileID int64 `json:"fileID"`
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

	fileInfo, err := dbwrapper.QueryFileInfo(req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// delete file from local storage
	var localPath = path.Join(GetBaseFolderPath(), fileInfo.Path)
	err = os.Remove(localPath)
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
