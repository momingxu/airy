package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort       = "8096"
	defaultDriver     = "sqlite"
	defaultSQLiteDSN  = "file:airy.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	postgresDSNFormat = "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s"
)

type Config struct {
	Port        string
	TLSCertFile string
	TLSKeyFile  string
	DockerMode  bool
	Database    DatabaseConfig
	Media       MediaConfig
}

type DatabaseConfig struct {
	Driver string
	DSN    string
}

type MediaConfig struct {
	FFmpegPath  string
	FFprobePath string
}

func Load() (Config, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	if driver == "" {
		driver = defaultDriver
		// Preserve existing PostgreSQL configurations that omit DB_DRIVER.
		for _, key := range []string{"DB_DSN", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
			if strings.TrimSpace(os.Getenv(key)) != "" {
				driver = "postgres"
				break
			}
		}
	}
	if driver != "postgres" && driver != "sqlite" {
		return Config{}, fmt.Errorf("DB_DRIVER 必须是 postgres 或 sqlite，当前值为 %q", driver)
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		if driver == "sqlite" {
			dsn = defaultSQLiteDSN
		} else {
			dsn = fmt.Sprintf(
				postgresDSNFormat,
				getEnv("DB_HOST", "localhost"),
				getEnv("DB_PORT", "5432"),
				getEnv("DB_USER", "postgres"),
				os.Getenv("DB_PASSWORD"),
				getEnv("DB_NAME", "airy"),
				getEnv("DB_SSLMODE", "disable"),
			)
		}
	}

	dockerMode, err := strconv.ParseBool(getEnv("IS_DOCKER", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("IS_DOCKER 必须是布尔值: %w", err)
	}

	tlsCertFile := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	tlsKeyFile := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if (tlsCertFile == "") != (tlsKeyFile == "") {
		return Config{}, fmt.Errorf("TLS_CERT_FILE 和 TLS_KEY_FILE 必须同时配置")
	}

	return Config{
		Port:        getEnv("PORT", defaultPort),
		TLSCertFile: tlsCertFile,
		TLSKeyFile:  tlsKeyFile,
		DockerMode:  dockerMode,
		Database: DatabaseConfig{
			Driver: driver,
			DSN:    dsn,
		},
		Media: MediaConfig{
			FFmpegPath:  getEnv("FFMPEG_PATH", "ffmpeg"),
			FFprobePath: getEnv("FFPROBE_PATH", "ffprobe"),
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
