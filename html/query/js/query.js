/*
 * @Author: shanghanjin
 * @Date: 2024-08-29 09:55:44
 * @LastEditTime: 2024-09-10 15:09:19
 * @FilePath: \UserFeedBack\html\query\js\query.js
 * @Description: 
 */
// 分页大小
const DEFAULT_PAGE_SIZE = 20;

// 尝试从sessionStorage中获取当前页码，如果没有则默认为0
let currentPageIndex = parseInt(sessionStorage.getItem('currentPageIndex')) || 0;

// 监听页面加载事件
document.addEventListener('DOMContentLoaded', function() {
    fetchData();
});
  
// 请求数据
function fetchData(pageIndex = currentPageIndex, pageSize = DEFAULT_PAGE_SIZE) {
    const params = new URLSearchParams({
        pageIndex: pageIndex.toString(),
        pageSize: pageSize.toString()
    });
    const url = `/api/queryFeedback?${params.toString()}`;

    // 配置 fetch 请求
    const options = {
        method: 'GET',
        // 对于 GET 请求，通常不需要设置 headers，除非有特定的需求
    };

    // 执行fetch
    fetch(url, options)
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            // 渲染返回结果
            renderFeedbackList(data);

            // 渲染分页按钮
            renderFeedbackPagination(data);

            // 存储当前请求的页面索引
            sessionStorage.setItem('currentPageIndex', data.currentPageIndex.toString());
        })
        .catch(error => console.error('Error fetching data:', error));
}

// 转换level为字符串
function frequencyString(level) {
    if (level === 0) {
        return '偶尔'
    }
    else if (level === 1) {
        return '经常'
    }
    else if (level === 2) {
        return '总是'
    }
    else {
        return ''
    }
}

// 格式化文件大小
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B'; // 如果字节数为0，则直接返回'0 B'

    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    let i = Math.floor(Math.log(bytes) / Math.log(1024)); // 计算索引，但不进行parseInt，因为要处理浮点数情况

    // 如果字节数小于1KB，则i为0，直接返回字节数 + 'B'
    if (i === 0) return `${bytes} ${sizes[i]}`;

    // 对于大于或等于1KB的情况，进行格式化
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
}

// 转换时间戳为字符串
function formatTimestamp(timestamp) {
    // 创建一个Date对象
    const date = new Date(timestamp);

    // 使用getFullYear(), getMonth() + 1, getDate(), getHours(), getMinutes(), getSeconds() 获取年月日时分秒
    // 注意：getMonth() 返回的是0-11，所以需要+1
    const year = date.getUTCFullYear();
    const month = String(date.getUTCMonth() + 1).padStart(2, '0'); // 使用padStart确保月份是两位数
    const day = String(date.getUTCDate()).padStart(2, '0');
    const hours = String(date.getUTCHours()).padStart(2, '0');
    const minutes = String(date.getUTCMinutes()).padStart(2, '0');
    const seconds = String(date.getUTCSeconds()).padStart(2, '0');

    // 拼接成YYYY-MM-DD HH:MM:SS格式
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

// 渲染请求数据
function renderFeedbackList(data) {
    const tbody = document.querySelector('#dataList tbody');
    tbody.innerHTML = ''; // 清空之前的列表项（如果有的话）
  
    data.pageData.forEach(item => {
        const row = tbody.insertRow();
        const cellModule = row.insertCell(0);
        const cellFreqency = row.insertCell(1);
        const cellDescription = row.insertCell(2);
        const cellSteps = row.insertCell(3);
        const cellAppVersion = row.insertCell(4);
        const cellTimeStamp = row.insertCell(5);
        const cellEmail = row.insertCell(6);
        const cellFile = row.insertCell(7);
        const cellOperate = row.insertCell(8);

        // 填充数据
        cellModule.textContent = item.impactedModule;
        cellFreqency.textContent = frequencyString(item.occurringFrequency);
        cellDescription.textContent = item.bugDescription;
        cellSteps.textContent = item.reproduceSteps;
        cellAppVersion.textContent = item.appVersion;
        cellTimeStamp.textContent = formatTimestamp(item.timeStamp);
        cellEmail.textContent = item.email

        // 创建文件列表的容器
        const fileList = document.createElement('ul');
        // 遍历文件数组并添加下载链接
        (item.files || []).forEach(file => {
            const fileItem = document.createElement('li');
            const fileDetails = document.createElement('span'); // 创建一个span来包含文件名和大小
            const fileName = document.createTextNode(file.fileName); // 创建文本节点用于文件名
            const fileSize = document.createTextNode(` (${formatFileSize(file.fileSize)})`); // 假设fileSize是字节数，你可以根据需要调整格式
          
            // 将文件名和大小添加到span中
            fileDetails.appendChild(fileName);
            fileDetails.appendChild(fileSize);

            const downloadLink = document.createElement('a');
            downloadLink.href = file.filePathOnOss;
            downloadLink.textContent = fileDetails.textContent; // 使用span的textContent作为链接的文本
            downloadLink.target = '_blank'; // 新标签页打开
            downloadLink.setAttribute('download', file.fileName);

            // 将链接添加到li元素中
            fileItem.appendChild(downloadLink);
            fileList.appendChild(fileItem);
        });

        // 将文件列表添加到 cellFile 单元格
        cellFile.appendChild(fileList);

        // 创建删除按钮
        const operationBtnDelete = document.createElement('button');
        operationBtnDelete.textContent = '删除';
        operationBtnDelete.onclick = function() {
            const body = JSON.stringify({ feedbackID: [parseInt(item.feedbackID, 10)] }); // 创建请求体
  
            // 发送POST请求
            fetch('/api/deleteFeedback', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json' // 设置请求头
                },
                body: body // 设置请求体
            })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Network response was not ok');
                }
                return response.text(); // 解析TEXT响应
            })
            .then(data => {
                console.log('Success:', data); // 处理成功的情况

                let rowCount = tbody.rows.length;
                let queryPageIndex = data.currentPageIndex;
                if (rowCount === 1) {
                    queryPageIndex -= 1;
                }
                if (queryPageIndex < 0) {
                    queryPageIndex = 0;
                }

                // 重新请求数据
                fetchData(queryPageIndex, DEFAULT_PAGE_SIZE);
            })
            .catch(error => {
                console.error('There has been a problem with your fetch operation:', error);
            });
        };
        cellOperate.appendChild(operationBtnDelete);
    });
}

// 渲染分页按钮
function renderFeedbackPagination(data) {
    const paginationDiv = document.getElementById('pagination');
    paginationDiv.innerHTML = ''; // 清空之前的分页按钮

    if (data.totalSize === 0) {
        return
    }

    // 向上取整分页数量
    var totalPages = Math.ceil(data.totalSize / DEFAULT_PAGE_SIZE);

    // 遍历页码并添加按钮
    for (let i = 1; i <= totalPages; i++) {
        const button = document.createElement('button');
        button.textContent = i;
        button.classList.add('page-button');
        button.onclick = function() {
            fetchData(this.textContent - 1, DEFAULT_PAGE_SIZE)
        };

        if (i - 1 === data.currentPageIndex) {
            button.disabled = true; // 禁用当前页码按钮
            button.classList.add('active'); // 添加活动类
        }

        paginationDiv.appendChild(button);
    }  
}