/*
 * @Author: shanghanjin
 * @Date: 2024-08-25 20:51:47
 * @LastEditTime: 2024-10-22 20:36:47
 * @FilePath: \CloudDisk\dbwrapper\db.go
 * @Description: 数据库操作封装
 */
package dbwrapper

import (
	"CloudDisk/configwrapper"
	"CloudDisk/dto"
	"CloudDisk/logwrapper"
	"errors"
	"path"

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
		CREATE TABLE IF NOT EXISTS folders (
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

		// 如果表为空则插入根目录，如果表不为空则检查根目录ID是否为1
		if isEmpty, err := isTableEmpty("folders"); err != nil {
			logwrapper.Logger.Fatalf("Failed to check if table is empty: %v", err)
		} else if isEmpty {
			_, err := db.Exec(`INSERT INTO folders (name, path, parent_folder_id) VALUES ('root', '/', NULL)`)
			if err != nil {
				logwrapper.Logger.Fatalf("Failed to insert root folder: %v", err)
			}
		} else {
			var id int
			row := db.QueryRow("SELECT id FROM folders ORDER BY id LIMIT 1")
			if err := row.Scan(&id); err != nil {
				logwrapper.Logger.Fatalf("Failed to scan first row: %v", err)
			}
			if id != 1 {
				logwrapper.Logger.Fatalf("Error: root folder ID is not 1")
			}
		}

		// 检查并创建触发器用于保护根目录不能被删除
		if isTriggerExist, err := isTriggerExists("protect_root_delete", "folders"); err != nil {
			logwrapper.Logger.Fatalf("Failed to check if trigger exists: %v", err)
		} else if !isTriggerExist {
			_, err := db.Exec(`
			CREATE TRIGGER protect_root_delete BEFORE DELETE ON folders
			FOR EACH ROW
			BEGIN
				IF OLD.id = 1 THEN
					SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Deletion of root directory is not allowed';
				END IF;
			END
			`)
			if err != nil {
				logwrapper.Logger.Fatalf("create trigger failed: %v", err)
			}
		}

		// 检查 files 表是否存在，如果不存在则创建它
		createTabFile := `
		CREATE TABLE IF NOT EXISTS files (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,  -- 文件唯一标识
			name VARCHAR(255) NOT NULL,            -- 文件名
			path VARCHAR(1024) NOT NULL,           -- 文件存储路径
			size BIGINT NOT NULL,                  -- 文件大小（以字节为单位）
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- 文件创建时间
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- 文件更新时间
			parent_folder_id BIGINT,                      -- 文件所属文件夹ID
			CONSTRAINT fk_folder FOREIGN KEY (parent_folder_id) REFERENCES folders(id) ON DELETE CASCADE  -- 文件夹ID外键，删除文件夹时级联删除文件
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
func QueryFolderContent(folderID int64) (*QueryFolderResult, error) {
	var (
		folders    = []dto.Folder{}
		files      = []dto.File{}
		rowsFolder *sql.Rows
		rowsFile   *sql.Rows
		err        error
	)

	query := "SELECT id, name, path, parent_folder_id, created_at, updated_at FROM folders WHERE parent_folder_id = ?;"
	if rowsFolder, err = db.Query(query, folderID); err != nil {
		return nil, err
	}
	defer rowsFolder.Close()

	for rowsFolder.Next() {
		var folder dto.Folder
		if err := rowsFolder.Scan(&folder.ID, &folder.Name, &folder.Path, &folder.ParentFolderID, &folder.CreatedAt, &folder.UpdatedAt); err != nil {
			return nil, err
		}

		folders = append(folders, folder)
	}

	query = "SELECT id, name, path, size, created_at, updated_at, parent_folder_id FROM files WHERE parent_folder_id = ?;"
	if rowsFile, err = db.Query(query, folderID); err != nil {
		return nil, err
	}
	defer rowsFile.Close()

	for rowsFile.Next() {
		var file dto.File
		if err := rowsFile.Scan(&file.ID, &file.Name, &file.Path, &file.Size, &file.CreatedAt, &file.UpdatedAt, &file.ParentFolderID); err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	return &QueryFolderResult{Folders: folders, Files: files}, nil
}

func QueryFolder(folderID int64) (*dto.Folder, error) {
	var (
		rowsFolder *sql.Rows
		err        error
	)

	query := "SELECT id, name, path, parent_folder_id, created_at, updated_at FROM folders WHERE id = ?;"
	if rowsFolder, err = db.Query(query, folderID); err != nil {
		return nil, err
	}
	defer rowsFolder.Close()

	for rowsFolder.Next() {
		var folder dto.Folder
		if err := rowsFolder.Scan(&folder.ID, &folder.Name, &folder.Path, &folder.ParentFolderID, &folder.CreatedAt, &folder.UpdatedAt); err != nil {
			return nil, err
		}

		return &folder, nil
	}

	return nil, errors.New("folder does not exist")
}

func QueryFile(fileID int64) (*dto.File, error) {
	var (
		rowsFile *sql.Rows
		err      error
	)

	query := "SELECT id, name, path, size, created_at, updated_at, parent_folder_id FROM files WHERE id = ?;"
	if rowsFile, err = db.Query(query, fileID); err != nil {
		return nil, err
	}
	defer rowsFile.Close()

	for rowsFile.Next() {
		var file dto.File
		if err := rowsFile.Scan(&file.ID, &file.Name, &file.Path, &file.Size, &file.CreatedAt, &file.UpdatedAt, &file.ParentFolderID); err != nil {
			return nil, err
		}

		return &file, nil
	}

	return nil, errors.New("file does not exist")
}

func QueryFolderPath(folderID int64) (string, error) {
	query := "SELECT path FROM folders WHERE id = ?;"
	row := db.QueryRow(query, folderID)

	var folderPath string
	err := row.Scan(&folderPath)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", errors.New("folder does not exist")
		}

		return "", err
	}

	return folderPath, err
}

func CreateFolder(folderName string, parentFolderID int64) (int64, error) {
	// 检查父文件夹是否存在
	if idExists, err := isIdExists(parentFolderID, "folders"); err != nil {
		return 0, err
	} else if !idExists {
		return 0, errors.New("parent folder does not exist")
	}

	pathParent, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return 0, err
	}

	folderPath := path.Join(pathParent, folderName)

	// 检查文件夹是否已存在，如果存在则走更新逻辑
	query := "SELECT id FROM folders WHERE path = ?;"
	row := db.QueryRow(query, folderPath)

	var folderID int64
	err = row.Scan(&folderID)
	if err != sql.ErrNoRows && err != nil {
		return 0, err
	}

	if err != sql.ErrNoRows {
		UpdateFolderUpdateTime(folderID)
		return folderID, nil
	}

	// 插入新文件夹
	query = "INSERT INTO folders (name, path, parent_folder_id) VALUES (?, ?, ?);"
	res, err := db.Exec(query, folderName, folderPath, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func CreateFile(fileName string, fileSize int64, parentFolderID int64) (int64, error) {
	// 检查父文件夹是否存在
	if idExists, err := isIdExists(parentFolderID, "folders"); err != nil {
		return 0, err
	} else if !idExists {
		return 0, errors.New("parent folder does not exist")
	}

	pathParent, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return 0, err
	}

	filePath := path.Join(pathParent, fileName)

	// 检查文件是否已存在，如果存在则走更新逻辑
	query := "SELECT id FROM files WHERE path = ?;"
	row := db.QueryRow(query, filePath)

	var fileID int64
	err = row.Scan(&fileID)
	if err != sql.ErrNoRows && err != nil {
		return 0, err
	}

	if err != sql.ErrNoRows {
		UpdateFileUpdateTime(fileID, fileSize)
		return fileID, nil
	}

	// 插入新文件
	query = "INSERT INTO files (name, path, size, parent_folder_id) VALUES (?, ?, ?, ?);"
	res, err := db.Exec(query, fileName, filePath, fileSize, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func RenameFolder(folderID int64, folderName string) error {
	if idExists, err := isIdExists(folderID, "folders"); err != nil {
		return err
	} else if !idExists {
		return errors.New("folder does not exist")
	}

	parentFolderID, err := QueryParentFolderID(folderID, "folders")
	if err != nil {
		return err
	}

	parentPath, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return err
	}

	newPath := path.Join(parentPath, folderName)

	query := "UPDATE folders SET name = ?, path = ? WHERE id = ?;"
	_, err = db.Exec(query, folderName, newPath, folderID)
	return err
}

func RenameFile(fileID int64, fileName string) error {
	if idExists, err := isIdExists(fileID, "files"); err != nil {
		return err
	} else if !idExists {
		return errors.New("file does not exist")
	}

	parentFolderID, err := QueryParentFolderID(fileID, "files")
	if err != nil {
		return err
	}

	parentPath, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return err
	}

	newPath := path.Join(parentPath, fileName)

	query := "UPDATE files SET name = ?, path = ? WHERE id = ?;"
	_, err = db.Exec(query, fileName, newPath, fileID)
	return err
}

func DeleteFolder(folderID int64) error {
	if idExists, err := isIdExists(folderID, "folders"); err != nil {
		return err
	} else if !idExists {
		return errors.New("folder does not exist")
	}

	query := "DELETE FROM folders WHERE id = ?;"
	_, err := db.Exec(query, folderID)
	return err
}

func DeleteFile(fileID int64) error {
	if idExists, err := isIdExists(fileID, "files"); err != nil {
		return err
	} else if !idExists {
		return errors.New("file does not exist")
	}

	query := "DELETE FROM files WHERE id = ?;"
	_, err := db.Exec(query, fileID)
	return err
}

func UpdateFolderUpdateTime(folderID int64) error {
	query := "UPDATE folders SET updated_at = NOW() WHERE id = ?;"
	_, err := db.Exec(query, folderID)
	return err
}

func UpdateFileUpdateTime(fileID int64, fileSize int64) error {
	query := "UPDATE files SET updated_at = NOW(), size = ? WHERE id = ?;"
	_, err := db.Exec(query, fileSize, fileID)
	return err
}

func isTableEmpty(tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s LIMIT 1);", tableName)
	row := db.QueryRow(query)

	var exists int
	err := row.Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 0, nil
}

func isTriggerExists(triggerName string, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_name = '%s' AND event_object_table = '%s');", triggerName, tableName)
	row := db.QueryRow(query)

	var exists int
	err := row.Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 1, nil
}

func isIdExists(id int64, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = ?);", tableName)
	row := db.QueryRow(query, id)

	var exists int
	err := row.Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 1, nil
}

func QueryParentFolderID(id int64, tableName string) (int64, error) {
	query := fmt.Sprintf("SELECT parent_folder_id FROM %s WHERE id = ?;", tableName)
	row := db.QueryRow(query, id)

	var parentFolderID int64
	err := row.Scan(&parentFolderID)
	return parentFolderID, err
}
