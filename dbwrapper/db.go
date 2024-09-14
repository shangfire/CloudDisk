/*
 * @Author: shanghanjin
 * @Date: 2024-08-25 20:51:47
 * @LastEditTime: 2024-09-13 14:38:49
 * @FilePath: \CloudDisk\dbwrapper\db.go
 * @Description: 数据库操作封装
 */
package dbwrapper

import (
	"CloudDisk/configwrapper"
	"CloudDisk/dto"
	"CloudDisk/logwrapper"

	"database/sql"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

var (
	// 数据库单例
	db *sql.DB
	// 单例标志
	once sync.Once
)

/**
 * @description: 初始化数据库连接
 * @return {*}
 */
func InitDB() {
	once.Do(func() {
		var err error
		// 连接到 MySQL 数据库
		address := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
			configwrapper.Cfg.Database.User,
			configwrapper.Cfg.Database.Password,
			configwrapper.Cfg.Database.Host,
			configwrapper.Cfg.Database.Port,
			configwrapper.Cfg.Database.Schema)
		db, err = sql.Open("mysql", address)
		if err != nil {
			logwrapper.Logger.Fatalf("Failed to connect to database: %v", err)
		}

		// 检查连接是否成功
		if err = db.Ping(); err != nil {
			logwrapper.Logger.Fatalf("Failed to ping database: %v", err)
		}

		// 检查 folders 表是否存在，如果不存在则创建它
		createTabFolder := `
		CREATE TABLE folders (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,  -- 文件夹唯一标识
			name VARCHAR(255) NOT NULL,            -- 文件夹名
			path VARCHAR(1024) NOT NULL,           -- 文件夹路径
			parent_folder_id BIGINT,               -- 父文件夹ID，根目录为NULL
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- 文件夹创建时间
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- 文件夹更新时间
			CONSTRAINT fk_parent_folder FOREIGN KEY (parent_folder_id) REFERENCES folders(id) ON DELETE CASCADE  -- 父文件夹外键，级联删除子文件夹
		);
		`

		if _, err := db.Exec(createTabFolder); err != nil {
			logwrapper.Logger.Fatalf("Failed to create table: %v", err)
		}

		// 检查 files 表是否存在，如果不存在则创建它
		createTabFile := `
		CREATE TABLE files (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,  -- 文件唯一标识
			name VARCHAR(255) NOT NULL,            -- 文件名
			path VARCHAR(1024) NOT NULL,           -- 文件存储路径
			size BIGINT NOT NULL,                  -- 文件大小（以字节为单位）
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- 文件创建时间
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- 文件更新时间
			parent_folder_id BIGINT,                      -- 文件所属文件夹ID
			CONSTRAINT fk_folder FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE  -- 文件夹ID外键，删除文件夹时级联删除文件
		);
		`

		if _, err := db.Exec(createTabFile); err != nil {
			logwrapper.Logger.Fatalf("Failed to create table: %v", err)
		}
	})
}

/**
 * @description: 关闭数据库连接
 * @return {*}
 */
func CloseDB() error {
	return db.Close()
}

type QueryFolderResult struct {
	Folders []dto.Folder `json:"folders"`
	Files   []dto.File   `json:"files"`
}

/**
 * @description: 查询文件夹下的所有文件夹和文件
 * @param {*int64} folderID 文件夹ID
 * @return {*}
 */
func QueryFolder(folderID *int64) (*QueryFolderResult, error) {
	var (
		folders    []dto.Folder
		files      []dto.File
		rowsFolder *sql.Rows
		rowsFile   *sql.Rows
		err        error
	)

	query := "SELECT id, name, path, parent_folder_id, created_at, updated_at FROM folders "

	if folderID != nil {
		query += "WHERE parent_folder_id = ?;"
		rowsFolder, err = db.Query(query, *folderID)
	} else {
		query += "WHERE parent_folder_id IS NULL;"
		rowsFolder, err = db.Query(query)
	}

	if err != nil {
		return nil, err
	}

	defer rowsFolder.Close()

	for rowsFolder.Next() {
		var folder dto.Folder
		var parentFolderID sql.NullInt64
		err := rowsFolder.Scan(&folder.ID, &folder.Name, &folder.Path, &parentFolderID, &folder.CreatedAt, &folder.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if parentFolderID.Valid {
			folder.ParentFolderID = new(int64)
			*folder.ParentFolderID = parentFolderID.Int64
		}

		folders = append(folders, folder)
	}

	query = "SELECT id, name, path, size, created_at, updated_at, parent_folder_id FROM files "

	if folderID != nil {
		query += "WHERE parent_folder_id = ?;"
		rowsFile, err = db.Query(query, *folderID)
	} else {
		query += "WHERE parent_folder_id IS NULL;"
		rowsFile, err = db.Query(query)
	}

	if err != nil {
		return nil, err
	}

	defer rowsFile.Close()

	for rowsFile.Next() {
		var file dto.File
		var parentFolderID sql.NullInt64
		err := rowsFile.Scan(&file.ID, &file.Name, &file.Path, &file.Size, &file.CreatedAt, &file.UpdatedAt, &parentFolderID)
		if err != nil {
			return nil, err
		}

		if parentFolderID.Valid {
			file.ParentFolderID = new(int64)
			*file.ParentFolderID = parentFolderID.Int64
		}

		files = append(files, file)
	}

	return &QueryFolderResult{Folders: folders, Files: files}, nil
}

func CreateFolder(folderName string, folderPath string, parentFolderID *int64) (int64, error) {
	query := "INSERT INTO folders (name, path, parent_folder_id) VALUES (?, ?, ?);"
	res, err := db.Exec(query, folderName, folderPath, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func CreateFile(fileName string, filePath string, fileSize int64, parentFolderID *int64) (int64, error) {
	query := "SELECT id FROM files WHERE path = ?;"
	row := db.QueryRow(query, filePath)

	var fileID int64
	err := row.Scan(&fileID)
	if err != nil {
		return 0, err
	}

	if fileID != 0 {
		UpdateFileUpdateTime(fileID)
		return fileID, nil
	}

	query = "INSERT INTO files (name, path, size, parent_folder_id) VALUES (?, ?, ?, ?);"
	res, err := db.Exec(query, fileName, filePath, fileSize, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func DeleteFile(fileID int64) error {
	query := "DELETE FROM files WHERE id = ?;"
	_, err := db.Exec(query, fileID)
	return err
}

func DeleteFolder(folderID int64) error {
	query := "DELETE FROM folders WHERE id = ?;"
	_, err := db.Exec(query, folderID)
	return err
}

func RenameFile(fileID int64, fileName string) error {
	query := "UPDATE files SET name = ? WHERE id = ?;"
	_, err := db.Exec(query, fileName, fileID)
	return err
}

func UpdateFileUpdateTime(fileID int64) error {
	query := "UPDATE files SET updated_at = NOW() WHERE id = ?;"
	_, err := db.Exec(query, fileID)
	return err
}
