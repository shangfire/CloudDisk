/*
 * @Author: shanghanjin
 * @Date: 2024-08-20 15:00:52
 * @LastEditTime: 2024-08-29 11:39:05
 * @FilePath: \UserFeedBack\dto\dto.go
 * @Description: 公共结构体
 */
package dto

import "time"

// FileType represents whether it's a folder or a file
type FileType int

const (
	FileTypeFolder FileType = iota // 0
	FileTypeFile
)

func (f FileType) String() string {
	switch f {
	case FileTypeFolder:
		return "folder"
	case FileTypeFile:
		return "file"
	default:
		return "unknown"
	}
}

type File struct {
	ID             int64     `json:"id"`
	ParentFolderID *int64    `json:"parentFolderId,omitempty"`
	Name           string    `json:"name"`
	Type           FileType  `json:"fileType"`
	Path           string    `json:"path"`
	Size           int64     `json:"size"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Folder struct {
	ID             int64     `json:"id"`
	ParentFolderID *int64    `json:"parentFolderId,omitempty"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
