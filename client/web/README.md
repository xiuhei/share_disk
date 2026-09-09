# Share Disk Web Client

Share Disk 的 Web 客户端界面，支持桌面端和移动端。

## 技术栈

- React 18
- TypeScript
- Tailwind CSS
- React Router
- Vite
- Lucide React (图标库)

## 功能模块

- 登录/注册
- 文件管理（上传、下载、预览、删除）
- 文件夹管理（创建、重命名、删除）
- 传输管理（上传/下载任务）
- 设备管理
- 回收站
- 分享链接管理
- 设置（账户、安全、通知、外观、存储）

## 快速开始

### 安装依赖

```bash
cd client/web
npm install
```

### 启动开发服务器

```bash
npm run dev
```

访问 http://localhost:3000

### 在手机上查看移动端界面

确保手机和开发电脑连接同一个局域网，然后运行：

```bash
cd client/web
npm run dev:mobile
```

在 Windows 中运行 `ipconfig`，找到当前网卡的 IPv4 地址。假设电脑地址为 `192.168.1.100`，在手机浏览器中打开：

```text
http://192.168.1.100:3000
```

首次访问时，如果 Windows 防火墙弹出提示，请只允许 Node.js 访问专用网络。也可以在桌面浏览器中打开开发者工具，切换到手机尺寸快速预览；真实手机更适合检查触摸区域、底部安全区和滚动体验。

### 构建生产版本

```bash
npm run build
```

构建产物将输出到 `dist/` 目录。

## 项目结构

```
client/web/
├── src/
│   ├── components/       # 通用组件
│   ├── pages/           # 页面组件
│   │   ├── Login/       # 登录页
│   │   ├── Files/       # 文件管理
│   │   ├── Transfers/   # 传输管理
│   │   ├── Devices/     # 设备管理
│   │   ├── Trash/       # 回收站
│   │   ├── Shares/      # 分享管理
│   │   └── Settings/    # 设置
│   ├── layouts/         # 布局组件
│   │   ├── DesktopLayout.tsx  # 桌面端布局
│   │   └── MobileLayout.tsx   # 移动端布局
│   ├── contexts/        # React Context
│   ├── hooks/           # 自定义 Hooks
│   ├── services/        # API 服务
│   ├── utils/           # 工具函数
│   └── styles/          # 全局样式
├── public/
├── index.html
├── package.json
├── vite.config.ts
├── tailwind.config.js
└── tsconfig.json
```

## 响应式设计

- **桌面端** (>=1025px): 侧边栏 + 顶栏布局
- **移动端** (<1025px): 底部导航栏布局

## 主题

支持浅色/深色主题切换，通过 `ThemeContext` 管理。

## API 集成

API 服务位于 `src/services/api.ts`，已实现基本的 API 调用方法。

实际使用时需要根据后端 API 进行调整。
