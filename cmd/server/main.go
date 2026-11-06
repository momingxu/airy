package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"airy/internal/config"
	"airy/internal/database"
	mediaservice "airy/internal/media"
	"airy/internal/scanner"

	"github.com/joho/godotenv"
	"github.com/soheilhy/cmux"
)

const dockerMediaRoot = "/mnt"

func main() {
	if err := godotenv.Overload(); err != nil && !os.IsNotExist(err) {
		slog.Warn("加载 .env 文件失败", "error", err)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置无效", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("连接数据库失败", "driver", cfg.Database.Driver, "error", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("获取数据库连接失败", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	mediaScanner := scanner.New(db, cfg.Media.FFprobePath, cfg.Media.FFmpegPath)
	mediaService := mediaservice.NewService(db)
	mediaRoots, rootsErr := discoverDockerMediaRoots(cfg.DockerMode, dockerMediaRoot)
	if rootsErr != nil {
		slog.Warn("发现 Docker 默认媒体目录失败", "root", dockerMediaRoot, "error", rootsErr)
	}
	registeredRoots := 0
	for _, mediaRoot := range mediaRoots {
		resolvedRoot, resolveErr := scanner.ResolveRoot(mediaRoot)
		if resolveErr != nil {
			slog.Warn("媒体目录不可用", "root", mediaRoot, "error", resolveErr)
			continue
		}
		directory := database.AssetDirectory{}
		if err := db.Where("directory_path = ?", resolvedRoot).FirstOrCreate(&directory, database.AssetDirectory{
			DirectoryName: filepath.Base(resolvedRoot),
			DirectoryPath: resolvedRoot,
		}).Error; err != nil {
			slog.Warn("登记媒体目录失败", "root", resolvedRoot, "error", err)
			continue
		}
		registeredRoots++
	}
	if cfg.DockerMode {
		slog.Info("Docker 媒体目录发现完成", "mount_root", dockerMediaRoot, "registered", registeredRoots)
	}

	automationContext, stopAutomation := context.WithCancel(context.Background())
	defer stopAutomation()
	if err := mediaScanner.StartAutomation(automationContext); err != nil {
		slog.Warn("启动目录自动化失败", "error", err)
	} else {
		defer mediaScanner.StopAutomation()
	}
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           newRouter(db, cfg.Database.Driver, mediaScanner, mediaService),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       60 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	serverErrors := make(chan error, 3)
	var redirectServer *http.Server
	var connectionMux cmux.CMux

	if cfg.TLSCertFile == "" {
		go func() {
			serverErrors <- server.ListenAndServe()
		}()
	} else {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			slog.Error("监听服务端口失败", "address", server.Addr, "error", err)
			os.Exit(1)
		}

		connectionMux = cmux.New(listener)
		connectionMux.SetReadTimeout(10 * time.Second)
		tlsListener := connectionMux.Match(cmux.TLS())
		httpListener := connectionMux.Match(cmux.Any())
		redirectServer = newHTTPSRedirectServer()

		go func() {
			if err := server.ServeTLS(tlsListener, cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
				serverErrors <- fmt.Errorf("HTTPS 服务: %w", err)
			}
		}()
		go func() {
			if err := redirectServer.Serve(httpListener); err != nil {
				serverErrors <- fmt.Errorf("HTTP 重定向服务: %w", err)
			}
		}()
		go func() {
			if err := connectionMux.Serve(); err != nil {
				serverErrors <- fmt.Errorf("连接协议复用器: %w", err)
			}
		}()
	}

	scheme := "http"
	if cfg.TLSCertFile != "" {
		scheme = "https"
	}
	slog.Info("API 服务已启动",
		"address", scheme+"://localhost:"+cfg.Port,
		"database", cfg.Database.Driver,
		"tls", cfg.TLSCertFile != "",
		"http_redirect", cfg.TLSCertFile != "",
	)

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("API 服务意外停止", "error", err)
			os.Exit(1)
		}
	case <-shutdownSignal.Done():
		slog.Info("正在关闭 API 服务")
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			slog.Error("优雅关闭服务失败", "error", err)
		}
		if redirectServer != nil {
			if err := redirectServer.Shutdown(shutdownContext); err != nil {
				slog.Error("关闭 HTTP 重定向服务失败", "error", err)
			}
		}
		if connectionMux != nil {
			connectionMux.Close()
		}
	}
}

func discoverDockerMediaRoots(dockerMode bool, mountRoot string) ([]string, error) {
	if !dockerMode {
		return nil, nil
	}

	entries, err := os.ReadDir(mountRoot)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
			continue
		}
		roots = append(roots, filepath.Join(mountRoot, entry.Name()))
	}
	return roots, nil
}

func newHTTPSRedirectServer() *http.Server {
	return &http.Server{
		Handler:           http.HandlerFunc(redirectToHTTPS),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func redirectToHTTPS(response http.ResponseWriter, request *http.Request) {
	if request.Host == "" {
		http.Error(response, "missing Host header", http.StatusBadRequest)
		return
	}

	target := "https://" + request.Host + request.URL.RequestURI()
	http.Redirect(response, request, target, http.StatusPermanentRedirect)
}
