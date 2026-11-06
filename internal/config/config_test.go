package config

import "testing"

func TestLoadDefaultsToSQLite(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DB_DSN", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("DB_SSLMODE", "")
	t.Setenv("FFPROBE_PATH", "")
	t.Setenv("FFMPEG_PATH", "")
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_KEY_FILE", "")
	t.Setenv("IS_DOCKER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	if cfg.Port != "8096" {
		t.Fatalf("expected default port 8096, got %q", cfg.Port)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected sqlite driver, got %q", cfg.Database.Driver)
	}
	if cfg.DockerMode {
		t.Fatal("expected Docker mode to be disabled by default")
	}
	expectedDSN := defaultSQLiteDSN
	if cfg.Database.DSN != expectedDSN {
		t.Fatalf("expected default SQLite DSN %q, got %q", expectedDSN, cfg.Database.DSN)
	}
}

func TestLoadRejectsUnsupportedDriver(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")

	if _, err := Load(); err == nil {
		t.Fatal("expected unsupported driver to return an error")
	}
}

func TestLoadSQLiteAndMediaConfig(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "")
	t.Setenv("FFPROBE_PATH", "/usr/bin/ffprobe")
	t.Setenv("FFMPEG_PATH", "/usr/bin/ffmpeg")
	t.Setenv("TLS_CERT_FILE", "/etc/airy/server.crt")
	t.Setenv("TLS_KEY_FILE", "/etc/airy/server.key")
	t.Setenv("IS_DOCKER", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.Database.Driver != "sqlite" || cfg.Database.DSN != defaultSQLiteDSN {
		t.Fatalf("unexpected SQLite config: %+v", cfg.Database)
	}
	if cfg.Media.FFprobePath != "/usr/bin/ffprobe" ||
		cfg.Media.FFmpegPath != "/usr/bin/ffmpeg" {
		t.Fatalf("unexpected media config: %+v", cfg.Media)
	}
	if cfg.TLSCertFile != "/etc/airy/server.crt" || cfg.TLSKeyFile != "/etc/airy/server.key" {
		t.Fatalf("unexpected TLS config: cert=%q key=%q", cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	if !cfg.DockerMode {
		t.Fatal("expected Docker mode to be enabled")
	}
}

func TestLoadRejectsIncompleteTLSConfig(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "/etc/airy/server.crt")
	t.Setenv("TLS_KEY_FILE", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete TLS configuration to return an error")
	}
}

func TestLoadRejectsInvalidDockerMode(t *testing.T) {
	t.Setenv("IS_DOCKER", "sometimes")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid IS_DOCKER to return an error")
	}
}

func TestDatabaseConfigurationSelection(t *testing.T) {
	cases := []struct {
		name   string
		env    map[string]string
		driver string
		dsn    string
	}{
		{"postgres fields", map[string]string{"DB_HOST": "database", "DB_USER": "airy"}, "postgres", "host=database port=5432 user=airy password= dbname=airy sslmode=disable"},
		{"postgres dsn", map[string]string{"DB_DSN": "postgres://user:password@database/airy"}, "postgres", "postgres://user:password@database/airy"},
		{"explicit postgres", map[string]string{"DB_DRIVER": "postgres"}, "postgres", "host=localhost port=5432 user=postgres password= dbname=airy sslmode=disable"},
		{"explicit sqlite overrides postgres fields", map[string]string{"DB_DRIVER": "sqlite", "DB_HOST": "database"}, "sqlite", defaultSQLiteDSN},
		{"custom sqlite", map[string]string{"DB_DRIVER": "sqlite", "DB_DSN": "file:/data/custom.db"}, "sqlite", "file:/data/custom.db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"DB_DRIVER", "DB_DSN", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE", "IS_DOCKER", "TLS_CERT_FILE", "TLS_KEY_FILE"} {
				t.Setenv(key, "")
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Database.Driver != tc.driver || cfg.Database.DSN != tc.dsn {
				t.Fatalf("unexpected config: %+v", cfg.Database)
			}
		})
	}
}
