package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/twistingmercury/mnemonic-api/internal/config"
)

func TestServerDoesNotConstructMCPListener(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve server test source path")
	}

	source, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "server.go"))
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}

	assert.NotContains(t, string(source), "internal/mcpserver")
	assert.NotContains(t, string(source), "NewMCPHTTPServer")
	assert.NotContains(t, string(source), "runMCPServer")
}

func TestCreateHTTPServerUsesAdminAPIConfiguration(t *testing.T) {
	t.Parallel()

	cfg := &config.MnemonicConfig{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         9090,
			ReadTimeout:  time.Second,
			WriteTimeout: 2 * time.Second,
			IdleTimeout:  3 * time.Second,
		},
	}

	srv := CreateHTTPServer(gin.New(), cfg)

	assert.Equal(t, "127.0.0.1:9090", srv.Addr)
	assert.Equal(t, cfg.Server.ReadTimeout, srv.ReadTimeout)
	assert.Equal(t, cfg.Server.WriteTimeout, srv.WriteTimeout)
	assert.Equal(t, cfg.Server.IdleTimeout, srv.IdleTimeout)
}
