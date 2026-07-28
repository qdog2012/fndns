package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fndns/manager/internal/demo"
	"github.com/fndns/manager/internal/httpapi"
	"github.com/fndns/manager/internal/provider"
	"github.com/fndns/manager/internal/secretbox"
	"github.com/fndns/manager/internal/service"
	"github.com/fndns/manager/internal/store"
)

//go:embed web/dist
var webAssets embed.FS

var version = "dev"

func main() {
	listen := flag.String("listen", "0.0.0.0:18788", "HTTP 监听地址")
	socketPath := flag.String("socket", "", "FNOS 统一网关 Unix Socket 路径")
	basePath := flag.String("base-path", "", "统一网关 URL 前缀")
	requireFNOSAdmin := flag.Bool("require-fnos-admin", false, "仅允许 FNOS 管理员访问")
	requireFNOSWebUI := flag.Bool("require-fnos-webui", false, "仅允许从 FNOS WebUI iframe 访问")
	dataDir := flag.String("data-dir", "./data", "数据目录")
	showVersion := flag.Bool("version", false, "显示版本")
	demoMode := flag.Bool("demo", false, "写入本地演示数据（仅用于开发）")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		logger.Error("创建数据目录失败", "error", err)
		os.Exit(1)
	}
	if err := os.Chmod(*dataDir, 0o700); err != nil {
		logger.Error("设置数据目录权限失败", "error", err)
		os.Exit(1)
	}
	box, err := secretbox.Open(filepath.Join(*dataDir, "master.key"))
	if err != nil {
		logger.Error("初始化凭据加密失败", "error", err)
		os.Exit(1)
	}
	databasePath := filepath.Join(*dataDir, "fndns.db")
	storage, err := store.Open(databasePath)
	if err != nil {
		logger.Error("初始化数据库失败", "error", err)
		os.Exit(1)
	}
	defer storage.Close()
	if err := os.Chmod(databasePath, 0o600); err != nil {
		logger.Error("设置数据库权限失败", "error", err)
		os.Exit(1)
	}
	_ = storage.CleanupLogs(context.Background())
	if *demoMode {
		if err := demo.Seed(context.Background(), storage, box); err != nil {
			logger.Error("写入演示数据失败", "error", err)
			os.Exit(1)
		}
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	svc := service.New(storage, box, provider.NewFactory(httpClient))
	assets, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		logger.Error("加载前端资源失败", "error", err)
		os.Exit(1)
	}
	handler := httpapi.New(svc, logger, assets)
	if *requireFNOSAdmin {
		handler = httpapi.RequireFNOSAdmin(handler)
	}
	if *requireFNOSWebUI {
		handler, err = httpapi.RequireFNOSWebUI(handler)
		if err != nil {
			logger.Error("初始化 FNOS WebUI 会话保护失败", "error", err)
			os.Exit(1)
		}
	}
	handler = httpapi.Mount(*basePath, handler)
	listeners, endpoints, err := openListeners(*listen, *socketPath)
	if err != nil {
		logger.Error("创建 HTTP 监听器失败", "error", err)
		os.Exit(1)
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	if *socketPath != "" {
		defer os.Remove(*socketPath)
	}
	server := &http.Server{Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second,
		IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		logger.Info("DNS 管理器已启动", "version", version, "listen", strings.Join(endpoints, ","), "base_path", *basePath, "fnos_admin_only", *requireFNOSAdmin, "fnos_webui_only", *requireFNOSWebUI)
		for _, listener := range listeners {
			listener := listener
			go func() {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					logger.Error("HTTP 服务监听异常", "listen", listener.Addr().String(), "error", err)
				}
			}()
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("停止服务失败", "error", err)
	}
}

func openListeners(tcpAddress, socketPath string) ([]net.Listener, []string, error) {
	listeners := make([]net.Listener, 0, 2)
	endpoints := make([]string, 0, 2)
	closeAll := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	if tcpAddress = strings.TrimSpace(tcpAddress); tcpAddress != "" {
		listener, err := net.Listen("tcp", tcpAddress)
		if err != nil {
			return nil, nil, err
		}
		listeners = append(listeners, listener)
		endpoints = append(endpoints, tcpAddress)
	}
	if strings.TrimSpace(socketPath) == "" {
		if len(listeners) == 0 {
			return nil, nil, fmt.Errorf("至少需要一个 HTTP 监听地址")
		}
		return listeners, endpoints, nil
	}
	cleanPath := filepath.Clean(socketPath)
	if info, err := os.Lstat(cleanPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			closeAll()
			return nil, nil, fmt.Errorf("Socket 路径已存在且不是 Socket: %s", cleanPath)
		}
		if err := os.Remove(cleanPath); err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("清理旧 Socket 失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		closeAll()
		return nil, nil, fmt.Errorf("检查 Socket 路径失败: %w", err)
	}
	listener, err := net.Listen("unix", cleanPath)
	if err != nil {
		closeAll()
		return nil, nil, err
	}
	// FNOS 的统一网关以独立账户连接 Socket；需与官方网关示例一致，
	// 允许该本机网关访问，而应用本身仍不会开放任何公网端口。
	if err := os.Chmod(cleanPath, 0o666); err != nil {
		_ = listener.Close()
		_ = os.Remove(cleanPath)
		closeAll()
		return nil, nil, fmt.Errorf("设置 Socket 权限失败: %w", err)
	}
	listeners = append(listeners, listener)
	endpoints = append(endpoints, "unix:"+cleanPath)
	return listeners, endpoints, nil
}
