package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	ubuntuapp "github.com/share-disk/share-disk/client/agent/app"
	"github.com/share-disk/share-disk/internal/config"
)

const desktopURL = "http://127.0.0.1:9191"

func main() {
	if err := run(); err != nil {
		log.Printf("Share Disk stopped: %v", err)
		messageBox("Share Disk 无法启动", err.Error())
		os.Exit(1)
	}
}

func run() error {
	dataRoot, err := localDataRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataRoot, 0700); err != nil {
		return fmt.Errorf("创建应用数据目录: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(dataRoot, "client.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("读取配置: %w", err)
	}
	applyWindowsDefaults(cfg, dataRoot)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置无效: %w", err)
	}
	migrations, err := findMigrations()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go openWhenReady(ctx, desktopURL)
	log.Printf("starting Share Disk at %s", desktopURL)
	return ubuntuapp.Run(ctx, cfg, migrations)
}

func localDataRoot() (string, error) {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("定位用户数据目录: %w", err)
		}
	}
	return filepath.Join(root, "ShareDisk"), nil
}

func applyWindowsDefaults(cfg *config.Config, dataRoot string) {
	if os.Getenv("SHARE_DISK_SQLITE_PATH") == "" {
		cfg.Database.SQLite = filepath.Join(dataRoot, "agent.db")
	}
	if os.Getenv("SHARE_DISK_STORAGE_ROOT") == "" {
		cfg.Agent.StorageRoot = filepath.Join(dataRoot, "storage")
	}
	if os.Getenv("SHARE_DISK_AGENT_SOCKET") == "" {
		cfg.Agent.SocketPath = filepath.Join(dataRoot, "agent.sock")
	}
	if os.Getenv("SHARE_DISK_DESKTOP_UI_ENABLED") == "" {
		cfg.Agent.DesktopUIEnabled = true
	}
	if os.Getenv("SHARE_DISK_DESKTOP_UI_HOST") == "" {
		cfg.Agent.DesktopUIHost = "127.0.0.1"
	}
	if os.Getenv("SHARE_DISK_DESKTOP_UI_PORT") == "" {
		cfg.Agent.DesktopUIPort = 9191
	}
}

func findMigrations() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		installed := filepath.Join(filepath.Dir(exe), "migrations", "sqlite")
		if _, statErr := os.Stat(filepath.Join(installed, "001_initial_schema.up.sql")); statErr == nil {
			return installed, nil
		}
	}
	development := filepath.Join("client", "ubuntu", "migrations", "sqlite")
	if _, err := os.Stat(filepath.Join(development, "001_initial_schema.up.sql")); err == nil {
		return development, nil
	}
	return "", fmt.Errorf("找不到数据库迁移文件，请重新安装 Share Disk")
}

func openWhenReady(ctx context.Context, url string) {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for i := 0; i < 40; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
		response, err := client.Get(url + "/api/status")
		if err != nil {
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode < 500 {
			command := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
			if err := command.Start(); err != nil {
				log.Printf("failed to open browser: %v", err)
			}
			return
		}
	}
}

func messageBox(title, message string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), 0x10)
}
