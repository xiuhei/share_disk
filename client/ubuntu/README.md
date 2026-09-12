# Share Disk Ubuntu 客户端

Ubuntu 客户端由系统级 Agent、CLI 和仅本机可访问的桌面管理界面组成。跨平台核心、CLI 与 SQLite 迁移位于 `client/agent/`，本目录只保留 Ubuntu 启动器及打包。它负责
SQLite 元数据、经过校验的文件内容、LAN 发现与传输，并通过 `.deb` 独立交付，
不属于 Docker Compose 服务端栈。

## 桌面界面

安装后访问 `http://127.0.0.1:9191`。Ubuntu 与 Windows 客户端共用同一套
UI/UE 资源，与 Web 桌面端保持一致：

- 统一的 224px 分组侧栏、顶部搜索、卡片、状态色、间距和深色模式；
- 多文件上传、实时搜索、下载和重命名；
- 移入回收站、恢复、永久删除及危险操作二次确认；
- 传输任务查看、取消和定时刷新；
- 设备 ID、Agent 版本、对象数量、存储路径与空间用量；
- 响应式窄屏布局、键盘焦点、`Esc` 关闭确认框和 `Ctrl+K` 搜索。

管理界面只绑定环回地址。文件上传后由 Agent 写入对象存储并校验，界面不会直接
访问任意宿主机路径。

## 构建安装包

在仓库根目录执行：

```bash
make deb
```

安装包输出到 `release/ubuntu/`。Debian 打包源位于 `client/ubuntu/packaging/deb/`，
中间文件写入被忽略的 `build/` 目录。

安装后可使用以下命令查看状态：

```bash
systemctl status share-disk-agent
curl http://127.0.0.1:9191/api/status
```

运行配置位于 `/etc/share-disk/agent.env`。默认数据保存在
`/var/lib/share-disk/`，本地 IPC socket 位于 `/run/share-disk/agent.sock`。
