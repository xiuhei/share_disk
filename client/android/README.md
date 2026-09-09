# Share Disk LAN Android

Android 10+ 原生客户端。它使用系统 NSD 自动发现 Ubuntu Agent，使用系统文件选择器（SAF）且不申请全盘存储权限；上传使用 tus 1.0 偏移恢复，下载使用 HTTP Range，完成时校验 SHA-256。文件列表支持重命名、7 天回收站、恢复和永久删除。

移动界面针对单手使用进行了优化：文件、传输、设备、我的四入口底部导航，默认直接进入文件库。搜索下方用同一行承载筛选、排序和列表/三列网格切换，避免工具控件挤占文件空间；统一添加面板和长按多选支持批量分享、移动和删除。离线文件明显置灰，但仍可查看大小、更新时间、上传设备和完整副本设备列表。

大文件上传和下载会作为 Android `dataSync` 前台任务运行，超过普通后台任务时间窗口时仍可继续。Android 13+ 会在第一次发起传输时请求通知权限；传输通知可返回任务页或直接取消任务。即使拒绝通知权限，任务仍会运行并可从系统任务管理器停止。

## 构建

```bash
export ANDROID_HOME=/path/to/android-sdk
export JAVA_HOME=/path/to/jdk-17
./client/android/gradlew -p client/android --no-daemon assembleDebug lintDebug
```

调试 APK 位于 `client/android/app/build/outputs/apk/debug/app-debug.apk`。正式包不在仓库保存签名私钥，需从环境变量注入：

Windows PowerShell：

```powershell
$env:JAVA_HOME = 'C:\Program Files\Microsoft\jdk-17.0.20.101-hotspot'
$env:ANDROID_HOME = "$env:LOCALAPPDATA\Android\Sdk"
.\client\android\gradlew.bat -p client\android --no-daemon assembleDebug
```

## 在手机上查看

这是原生 Android 界面，不通过浏览器访问。开启手机的“开发者选项”和“USB 调试”，连接电脑后执行：

```powershell
& "$env:LOCALAPPDATA\Android\Sdk\platform-tools\adb.exe" devices
& "$env:LOCALAPPDATA\Android\Sdk\platform-tools\adb.exe" install -r .\client\android\app\build\outputs\apk\debug\app-debug.apk
```

也可以直接把 `app-debug.apk` 发送到 Android 10+ 手机，在系统文件管理器中允许“安装未知应用”后安装。首次进入时配置控制服务器地址并初始化或登录设备。

```bash
export SHARE_DISK_ANDROID_KEYSTORE=/absolute/path/release.jks
export SHARE_DISK_ANDROID_KEYSTORE_PASSWORD='...'
export SHARE_DISK_ANDROID_KEY_ALIAS='...'
export SHARE_DISK_ANDROID_KEY_PASSWORD='...'
make apk
```

签名后的正式 APK 统一输出到 `release/android/`。

## 当前产品边界

- LAN V1 是单账号、单 Ubuntu Agent、一个首 Android 设备的可侧载版本。
- 首次使用选择“初始化”，后续用已保存的 device ID 登录。
- 自动发现只是候选地址，认证仍由 Control Server 签发的短期 Access Token 完成；mDNS 不可用时保留手工 Agent URL。
- 调试闭环允许可信隔离 LAN 上的 HTTP；正式网络必须在 Control Server 和 Agent 前提供受信 TLS 入口。
