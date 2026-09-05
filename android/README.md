# Share Disk LAN Android

Android 10+ 原生客户端。它使用系统 NSD 自动发现 Ubuntu Agent，使用系统文件选择器（SAF）且不申请全盘存储权限；上传使用 tus 1.0 偏移恢复，下载使用 HTTP Range，完成时校验 SHA-256。文件列表支持重命名、7 天回收站、恢复和永久删除。

## 构建

```bash
export ANDROID_HOME=/path/to/android-sdk
export JAVA_HOME=/path/to/jdk-17
./gradlew --no-daemon assembleDebug lintDebug
```

调试 APK 位于 `app/build/outputs/apk/debug/app-debug.apk`。正式包不在仓库保存签名私钥，需从环境变量注入：

```bash
export SHARE_DISK_ANDROID_KEYSTORE=/absolute/path/release.jks
export SHARE_DISK_ANDROID_KEYSTORE_PASSWORD='...'
export SHARE_DISK_ANDROID_KEY_ALIAS='...'
export SHARE_DISK_ANDROID_KEY_PASSWORD='...'
./gradlew --no-daemon assembleRelease lintRelease
```

## 当前产品边界

- LAN V1 是单账号、单 Ubuntu Agent、一个首 Android 设备的可侧载版本。
- 首次使用选择“初始化”，后续用已保存的 device ID 登录。
- 自动发现只是候选地址，认证仍由 Control Server 签发的短期 Access Token 完成；mDNS 不可用时保留手工 Agent URL。
- 调试闭环允许可信隔离 LAN 上的 HTTP；正式网络必须在 Control Server 和 Agent 前提供受信 TLS 入口。
