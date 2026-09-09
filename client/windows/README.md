# Share Disk Windows 客户端

Windows 客户端由原生后台 Agent 和本机环回桌面界面组成。文件内容、SQLite
目录和日志均保存在当前用户的 `%LOCALAPPDATA%\ShareDisk`，界面仅监听
`127.0.0.1:9191`，不会把未认证的管理界面暴露到局域网。

## 已实现功能

- 与 Web 桌面端一致的侧栏、色彩、间距、卡片、按钮和深色模式；
- 多文件上传、文件列表和搜索；
- 校验后下载、重命名、移入回收站、恢复和永久删除；
- 传输任务列表、自动刷新和取消传输；
- 本机设备状态、版本、对象数量、存储用量和存储路径；
- 删除、永久删除、取消传输等危险操作的二次确认；
- 单实例存储锁和仅本机可访问的 HTTP 安全边界；
- 可选的控制服务器、LAN API 和设备发现配置（沿用 Agent 环境变量）。

## 开发运行

在仓库根目录执行：

```powershell
go run ./client/windows/cmd/share-disk
```

应用启动后会使用系统默认浏览器打开桌面界面。退出开发进程会安全关闭 Agent。

## 构建发布包

```powershell
./client/windows/build.ps1 -Version 0.1.0
```

输出：

- `release/windows/ShareDisk/ShareDisk.exe`
- `release/windows/ShareDisk_<version>_windows_amd64.zip`

发布目录中的 `migrations` 必须和 `ShareDisk.exe` 一起分发。客户端首次启动会自动
创建数据库，不要求管理员权限。

## 可选环境变量

默认值可以用现有 `SHARE_DISK_*` Agent 环境变量覆盖。常用配置包括：

- `SHARE_DISK_CONTROL_URL`
- `SHARE_DISK_LAN_ENABLED`
- `SHARE_DISK_LAN_HOST` / `SHARE_DISK_LAN_PORT`
- `SHARE_DISK_LAN_ADVERTISE_URL`
- `SHARE_DISK_ACCESS_PUBLIC_KEY` / `SHARE_DISK_ACCESS_PUBLIC_KEY_FILE`
- `SHARE_DISK_STORAGE_ROOT`
- `SHARE_DISK_SQLITE_PATH`
- `SHARE_DISK_DESKTOP_UI_PORT`

启用控制服务器协调时，必须同时配置 LAN API、可访问的 LAN 地址和访问令牌公钥；
配置不完整时客户端会拒绝启动并显示明确错误。
