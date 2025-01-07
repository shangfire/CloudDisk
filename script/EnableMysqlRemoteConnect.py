"""
Author: shanghanjin
Date: 2024-09-11 15:31:37
LastEditTime: 2024-09-11 16:15:22
Description:
"""

import pymysql
import json
import os


def load_db_config(file_path):
    """
    description: 获取配置文件中的数据库连接信息
    param {*} file_path 配置文件路径
    return {*} 数据库连接信息
    """

    # 获取当前脚本的完整路径
    current_file_path = os.path.abspath(__file__)
    # 获取当前脚本所在的目录
    current_dir = os.path.dirname(current_file_path)
    # 构建目标文件的完整路径
    full_path = os.path.join(current_dir, file_path)

    # 判断文件是否存在
    if not os.path.exists(full_path):
        raise FileNotFoundError(f"Configuration file '{full_path}' does not exist.")

    with open(full_path, "r") as file:
        config = json.load(file)

        # 检查配置文件中是否包含 "database" 顶级字段
        if "database" not in config:
            raise ValueError("Configuration file is missing the 'database' section.")

        db_config = config["database"]

        # 检查 "database" 部分中的必要字段
        required_fields = ["host", "port", "user", "password"]

        for field in required_fields:
            if field not in db_config:
                raise ValueError(
                    f"'database' section is missing required field: '{field}'"
                )

        return db_config


def set_remote_access(config, enable):
    """
    description:
    param {*} config config文件中的数据库连接信息
    param {*} enable 是否开启远程连接
    return {*}
    """

    connection = None
    try:
        user = config["user"]
        password = config["password"]
        database = "mysql"

        # 连接到数据库
        connection = pymysql.connect(
            # 尝试使用host="localhost"和user="root"时，总是被拒绝访问，提示::1或者127.0.0.1被拒绝访问，实际上此时数据库里的user表user对应的是localhost
            # 用多种解决方法，比如在my.cnf里添加绑定，或者在数据库表里添加::1和127.0.0.1，这里临时强制使用unix_socket连接
            # 数据库如果不关闭远程连接，总是会被远程莫名关闭，所以仅限调试时打开
            unix_socket="/usr/local/mysql9/tmp/mysql.sock",
            user=user,
            password=password,
            database=database,
        )

        with connection.cursor() as cursor:
            if enable:
                # 开启远程连接权限（允许 user 用户从任何主机连接）
                cursor.execute(f"update user set host='%' where user='{user}';")
            else:
                # 关闭远程连接权限（只允许 user 用户从本地主机连接）
                cursor.execute(f"update user set host='localhost' where user='{user}';")

            # 刷新权限，确保变更生效
            cursor.execute("FLUSH PRIVILEGES;")
            print(f"Remote access {'enabled' if enable else 'disabled'} successfully.")

    finally:
        connection.close()


if __name__ == "__main__":
    # 读取配置
    config = load_db_config("../config")

    # 根据用户输入决定开启或关闭远程连接
    user_input = (
        input("Enter '1' to open remote access, or '0' to close it: ").strip().lower()
    )

    if user_input == "1":
        set_remote_access(config, enable=True)
    elif user_input == "0":
        set_remote_access(config, enable=False)
    else:
        print("Invalid input! Please enter '1' or '0'.")
