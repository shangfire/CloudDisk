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
	type UploadFileRequest struct {
		FileName       string `json:"fileName"`
		FileSize       int64  `json:"fileSize"`
		ParentFolderID *int64 `json:"parentFolderID,omitempty"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// parse the request body
	var req UploadFileRequest
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

	fullPath := filepath.Join(GetBaseFolderPath(), parentFolderPath, req.FileName)
	MkPathFolder(fullPath)

	fileWrite, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fileWrite.Close()

	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			_, err := fileWrite.Write(buf[:n])
			if err != nil {
				http.Error(w, "Error writing to file", http.StatusInternalServerError)
				return
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
	}

	_, err = dbwrapper.CreateFile(req.FileName, fullPath, req.FileSize, req.ParentFolderID)
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
