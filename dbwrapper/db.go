/*
 * @Author: shanghanjin
 * @Date: 2024-08-25 20:51:47
 * @LastEditTime: 2025-01-17 17:07:19
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
 * @return
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

		// 检查 folders 表是否存在，如果不存在则创建
		createTabFolder := `
		CREATE TABLE IF NOT EXISTS folders (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,  -- 文件夹唯一标识
			name VARCHAR(255) NOT NULL,            -- 文件夹名
			path VARCHAR(1024) NOT NULL,           -- 文件夹路径
			parent_folder_id BIGINT,               -- 父文件夹ID,根目录为NULL
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- 文件夹创建时间
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- 文件夹更新时间
			CONSTRAINT fk_parent_folder FOREIGN KEY (parent_folder_id) REFERENCES folders(id) ON DELETE CASCADE  -- 父文件夹外键,级联删除子文件夹
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
		// 创建触发器时如果不指定DEFINER则会使用当前的DEFINER，如果当前是root@%，后续该账户又被删除了或者被修改为root@localhost，则触发器会报错
		if isTriggerExist, err := triggerExist("protect_root_delete", "folders"); err != nil {
			logwrapper.Logger.Fatalf("Failed to check if trigger exists: %v", err)
		} else if !isTriggerExist {
			definerUser := configwrapper.Cfg.Database.User
			definerHost := "localhost"
			definerClause := fmt.Sprintf("DEFINER=`%s`@`%s`", definerUser, definerHost)

			// 构建带有DEFINER子句的创建触发器SQL语句
			createTriggerSQL := fmt.Sprintf(`
				CREATE %s TRIGGER protect_root_delete BEFORE DELETE ON folders
				FOR EACH ROW
				BEGIN
					IF OLD.id = 1 THEN
						SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Deletion of root directory is not allowed';
					END IF;
				END
			`, definerClause)

			_, err := db.Exec(createTriggerSQL)
			if err != nil {
				logwrapper.Logger.Fatalf("create trigger failed: %v", err)
			}
		}

		// 检查 files 表是否存在，如果不存在则创建
		createTabFile := `
		CREATE TABLE IF NOT EXISTS files (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,  -- 文件唯一标识
			name VARCHAR(255) NOT NULL,            -- 文件名
			path VARCHAR(1024) NOT NULL,           -- 文件存储路径
			size BIGINT NOT NULL,                  -- 文件大小（以字节为单位）
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- 文件创建时间
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,  -- 文件更新时间
			parent_folder_id BIGINT,                      -- 文件所属文件夹ID
			CONSTRAINT fk_folder FOREIGN KEY (parent_folder_id) REFERENCES folders(id) ON DELETE CASCADE  -- 文件夹ID外键,删除文件夹时级联删除文件
		);
		`

		if _, err := db.Exec(createTabFile); err != nil {
			logwrapper.Logger.Fatalf("Failed to create table: %v", err)
		}
	})
}

/**
 * @description: 关闭数据库连接
 * @return
 */
func CloseDB() error {
	return db.Close()
}

type QueryFolderResult struct {
	Self    *dto.Folder  `json:"self"`
	Folders []dto.Folder `json:"folders"`
	Files   []dto.File   `json:"files"`
}

/**
 * @description: 查询文件夹信息，包括文件夹本身信息和所有子文件夹&子文件信息
 * @param {int64} folderID 文件夹ID
 * @return {*} QueryFolderResult 被查询信息
 */
func QueryFolderInfoFull(folderID int64) (*QueryFolderResult, error) {
	var (
		folders    = []dto.Folder{}
		files      = []dto.File{}
		rowsFolder *sql.Rows
		rowsFile   *sql.Rows
		err        error
	)

	// 查询文件夹信息
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

	// 查询文件信息
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

	// 查询自己本身信息
	var self *dto.Folder
	if self, err = QueryFolderInfo(folderID); err != nil {
		return nil, err
	}

	return &QueryFolderResult{Self: self, Folders: folders, Files: files}, nil
}

/**
 * @description: 查询文件夹信息
 * @param {int64} folderID 文件夹id
 * @return {*} dto.Folder 被查询信息
 */
func QueryFolderInfo(folderID int64) (*dto.Folder, error) {
	var folder dto.Folder
	var parentFolderID sql.NullInt64
	query := "SELECT id, name, path, parent_folder_id, created_at, updated_at FROM folders WHERE id = ?;"
	err := db.QueryRow(query, folderID).Scan(&folder.ID, &folder.Name, &folder.Path, &parentFolderID, &folder.CreatedAt, &folder.UpdatedAt)
	if err == sql.ErrNoRows {
		// 如果没有找到记录，返回错误
		return nil, errors.New("folder does not exist")
	} else if err != nil {
		// 其他错误情况
		return nil, err
	}

	// 如果是根目录，因为根目录的parentID是null，所以需要手动设置为0
	if parentFolderID.Valid {
		folder.ParentFolderID = parentFolderID.Int64
	} else {
		folder.ParentFolderID = 0
	}
	return &folder, nil
}

/**
 * @description: 查询文件信息
 * @param {int64} fileID 文件id
 * @return {*} dto.File 被查询信息
 */
func QueryFileInfo(fileID int64) (*dto.File, error) {
	var file dto.File
	query := "SELECT id, name, path, size, created_at, updated_at, parent_folder_id FROM files WHERE id = ?;"

	// 使用 QueryRow 替代 Query，因为我们期望只有一个结果
	err := db.QueryRow(query, fileID).Scan(&file.ID, &file.Name, &file.Path, &file.Size, &file.CreatedAt, &file.UpdatedAt, &file.ParentFolderID)
	if err == sql.ErrNoRows {
		// 如果没有找到记录，返回错误
		return nil, errors.New("file does not exist")
	} else if err != nil {
		// 其他错误情况
		return nil, err
	}

	return &file, nil
}

/**
 * @description: 查询文件夹路径
 * @param {int64} folderID 文件夹ID
 * @return {string} 文件夹路径
 */
func QueryFolderPath(folderID int64) (string, error) {
	var folderPath string
	query := "SELECT path FROM folders WHERE id = ?;"

	err := db.QueryRow(query, folderID).Scan(&folderPath)
	if err == sql.ErrNoRows {
		return "", errors.New("folder does not exist")
	} else if err != nil {
		return "", err
	}

	return folderPath, err
}

/**
 * @description: 创建文件夹
 * @param {string} folderName 文件夹名称
 * @param {int64} parentFolderID 父文件夹ID
 * @return {int64} 新建文件夹ID
 */
func CreateFolder(folderName string, parentFolderID int64) (int64, error) {
	// 检查父文件夹是否存在
	if idExist, err := FolderExistByID(parentFolderID); err != nil {
		return 0, err
	} else if !idExist {
		return 0, errors.New("parent folder does not exist")
	}

	// 查询父文件夹路径
	pathParent, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return 0, err
	}

	// 拼接文件夹路径
	folderPath := path.Join(pathParent, folderName)

	// 检查文件夹是否已存在，如果存在则失败
	if pathExist, err := FolderExistByPath(folderPath); err != nil {
		return 0, err
	} else if pathExist {
		return 0, errors.New("folder already exists")
	}

	// 插入新文件夹
	query := "INSERT INTO folders (name, path, parent_folder_id) VALUES (?, ?, ?);"
	res, err := db.Exec(query, folderName, folderPath, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

/**
 * @description: 新建文件
 * @param {string} fileName 文件名
 * @param {int64} fileSize 文件大小
 * @param {int64} parentFolderID 父文件夹ID
 * @return {int64} 新建文件ID
 */
func CreateFile(fileName string, fileSize int64, parentFolderID int64) (int64, error) {
	// 检查父文件夹是否存在
	if idExist, err := FolderExistByID(parentFolderID); err != nil {
		return 0, err
	} else if !idExist {
		return 0, errors.New("parent folder does not exist")
	}

	// 查询父文件夹路径
	pathParent, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return 0, err
	}

	// 拼接文件路径
	filePath := path.Join(pathParent, fileName)

	// 检查文件是否已存在，如果存在则失败
	if pathExist, err := FileExistByPath(filePath); err != nil {
		return 0, err
	} else if pathExist {
		return 0, errors.New("file already exists")
	}

	// 插入新文件
	query := "INSERT INTO files (name, path, size, parent_folder_id) VALUES (?, ?, ?, ?);"
	res, err := db.Exec(query, fileName, filePath, fileSize, parentFolderID)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

/**
 * @description: 重命名文件夹
 * @param {int64} folderID 文件夹ID
 * @param {string} folderName 文件夹名称
 * @return
 */
func RenameFolder(folderID int64, folderNewName string) error {
	// 检查文件夹是否存在
	if idExists, err := FolderExistByID(folderID); err != nil {
		return err
	} else if !idExists {
		return errors.New("folder does not exist")
	}

	// 查询父文件夹ID
	parentFolderID, err := QueryParentFolderID(folderID, "folders")
	if err != nil {
		return err
	}

	// 查询父文件夹路径
	parentPath, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return err
	}

	// 拼接新文件夹路径
	newPath := path.Join(parentPath, folderNewName)

	// 更新文件夹名称和路径
	query := "UPDATE folders SET name = ?, path = ? WHERE id = ?;"
	_, err = db.Exec(query, folderNewName, newPath, folderID)
	return err
}

/**
 * @description: 重命名文件
 * @param {int64} fileID 文件ID
 * @param {string} fileName 文件名称
 * @return
 */
func RenameFile(fileID int64, fileNewName string) error {
	// 检查文件是否存在
	if idExists, err := FileExistByID(fileID); err != nil {
		return err
	} else if !idExists {
		return errors.New("file does not exist")
	}

	// 查询父文件夹ID
	parentFolderID, err := QueryParentFolderID(fileID, "files")
	if err != nil {
		return err
	}

	// 查询父文件夹路径
	parentPath, err := QueryFolderPath(parentFolderID)
	if err != nil {
		return err
	}

	// 拼接新文件路径
	newPath := path.Join(parentPath, fileNewName)

	// 更新文件名称和路径
	query := "UPDATE files SET name = ?, path = ? WHERE id = ?;"
	_, err = db.Exec(query, fileNewName, newPath, fileID)
	return err
}

/**
 * @description: 删除文件夹
 * @param {int64} folderID 文件夹ID
 * @return
 */
func DeleteFolder(folderID int64) error {
	// 检查文件夹是否存在
	if idExists, err := FolderExistByID(folderID); err != nil {
		return err
	} else if !idExists {
		return errors.New("folder does not exist")
	}

	// 删除文件夹，级联关系保证了子文件夹和文件也会被删除
	query := "DELETE FROM folders WHERE id = ?;"
	_, err := db.Exec(query, folderID)
	return err
}

/**
 * @description: 删除文件
 * @param {int64} fileID 文件ID
 * @return
 */
func DeleteFile(fileID int64) error {
	// 检查文件是否存在
	if idExists, err := FileExistByID(fileID); err != nil {
		return err
	} else if !idExists {
		return errors.New("file does not exist")
	}

	// 删除文件
	query := "DELETE FROM files WHERE id = ?;"
	_, err := db.Exec(query, fileID)
	return err
}

/**
 * @description: 查询父文件夹ID
 * @param {int64} id 文件/夹ID
 * @param {string} tableName 表名
 * @return {int64} 父文件夹ID
 */
func QueryParentFolderID(id int64, tableName string) (int64, error) {
	query := fmt.Sprintf("SELECT parent_folder_id FROM %s WHERE id = ?;", tableName)
	row := db.QueryRow(query, id)

	var parentFolderID int64
	err := row.Scan(&parentFolderID)
	return parentFolderID, err
}

/**
 * @description: 更新文件夹时间
 * @param {int64} folderID 文件夹ID
 * @return
 */
func UpdateFolderUpdateTime(folderID int64) error {
	query := "UPDATE folders SET updated_at = NOW() WHERE id = ?;"
	_, err := db.Exec(query, folderID)
	return err
}

/**
 * @description: 文件夹路径是否存在
 * @param {string} path 文件夹路径
 * @return {bool} 是否存在
 */
func FolderExistByPath(path string) (bool, error) {
	return pathExist(path, "folders")
}

/**
 * @description: 文件路径是否存在
 * @param {string} path 文件路径
 * @return {bool} 是否存在
 */
func FileExistByPath(path string) (bool, error) {
	return pathExist(path, "files")
}

/**
 * @description: 文件夹ID是否存在
 * @param {int64} folderID 文件夹ID
 * @return {bool} 是否存在
 */
func FolderExistByID(folderID int64) (bool, error) {
	return idExist(folderID, "folders")
}

/**
 * @description: 文件ID是否存在
 * @param {int64} fileID 文件ID
 * @return {bool} 是否存在
 */
func FileExistByID(fileID int64) (bool, error) {
	return idExist(fileID, "files")
}

/**
 * @description: 更新文件更新时间和大小
 * @param {int64} fileID 文件ID
 * @param {int64} fileSize 文件大小
 * @return
 */
func UpdateFileUpdateTimeAndSize(fileID int64, fileSize int64) error {
	query := "UPDATE files SET updated_at = NOW(), size = ? WHERE id = ?;"
	_, err := db.Exec(query, fileSize, fileID)
	return err
}

func isTableEmpty(tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s LIMIT 1)", tableName)
	var exists int

	// EXISTS总是会返回一个值，即使表为空，所以不用检查sql.ErrNoRows
	err := db.QueryRow(query).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 0, nil
}

func triggerExist(triggerName string, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_name = '%s' AND event_object_table = '%s');", triggerName, tableName)
	var exists int

	err := db.QueryRow(query).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 1, nil
}

func idExist(id int64, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = ?);", tableName)
	var exists int

	err := db.QueryRow(query, id).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 1, nil
}

func pathExist(path string, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE path = ?);", tableName)
	var exists int

	err := db.QueryRow(query, path).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists == 1, nil
}
