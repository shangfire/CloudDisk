package business

import (
	"CloudDisk/configwrapper"
	"CloudDisk/dbwrapper"
	"CloudDisk/logwrapper"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 从表单中获取其他字段（如果有的话）
	parentFolderIDStr := r.FormValue("parentFolderID")
	parentFolderID, err := strconv.ParseInt(parentFolderIDStr, 10, 64)
	if err != nil && parentFolderIDStr != "" {
		http.Error(w, "Invalid parentFolderID", http.StatusBadRequest)
		return
	}

	fileName := r.FormValue("fileName")
	if fileName == "" {
		http.Error(w, "Error retrieving the fileName", http.StatusBadRequest)
		return
	}

	parentFolderPath, err := dbwrapper.QueryFolderPath(&parentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fullPath := filepath.Join(GetBaseFolderPath(), parentFolderPath, handler.Filename)
	MkPathFolder(fullPath)

	fileWrite, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fileWrite.Close()

	// 复制文件内容
	if _, err := io.Copy(fileWrite, file); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = dbwrapper.CreateFile(handler.Filename, fullPath, 0, &parentFolderID)
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

	exeFolder := filepath.Dir(exePath)
	baseFolderPath := filepath.Join(exeFolder, configwrapper.Cfg.Local.BaseFolder)
	return baseFolderPath
}

func MkPathFolder(path string) {
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
}

func MakeAbsoluteFolder(relativePath string) {
	exePath, err := os.Executable()
	if err != nil {
		logwrapper.Logger.Fatal(err)
	}

	exeFolder := filepath.Dir(exePath)
	absoluteFolder := filepath.Join(exeFolder, configwrapper.Cfg.Local.BaseFolder, relativePath)
	os.MkdirAll(absoluteFolder, os.ModePerm)
}
