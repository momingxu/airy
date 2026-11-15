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
	"syscall"
	"time"

	"airy/internal/config"
	"airy/internal/database"
	mediaservice "airy/internal/media"
	"airy/internal/scanner"

	"github.com/joho/godotenv"
	"github.com/soheilhy/cmux"
)

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
