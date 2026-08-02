// HeyCode 后端入口。
//
// 装配依赖链：config → db → stores → sshPool/eventbus/runner → services → handlers → router → http.Server
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heycode/backend-go/internal/config"
	"github.com/heycode/backend-go/internal/crypto"
	"github.com/heycode/backend-go/internal/db"
	"github.com/heycode/backend-go/internal/eventbus"
	"github.com/heycode/backend-go/internal/runner"
	"github.com/heycode/backend-go/internal/service"
	"github.com/heycode/backend-go/internal/ssh"
	"github.com/heycode/backend-go/internal/store"
	httptransport "github.com/heycode/backend-go/internal/transport/http"
	"github.com/heycode/backend-go/internal/transport/ws"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("heycode-backend: %v", err)
	}
}

func run() error {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.IsMasterKeyPlaceholder() {
		log.Printf("警告: MASTER_KEY 为占位符，使用临时内存密钥（重启后加密数据失效，仅用于本地调试）")
	}

	// 2. 解析主密钥
	masterKey, err := crypto.ParseMasterKey(cfg.MasterKey)
	if err != nil {
		// 占位符或非法 key：生成临时密钥
		if cfg.IsMasterKeyPlaceholder() {
			masterKey = make([]byte, 32)
			for i := range masterKey {
				masterKey[i] = byte(i)
			}
		} else {
			return fmt.Errorf("parse master key: %w", err)
		}
	}

	// 3. 打开数据库 + 迁移
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	log.Printf("数据库已就绪: %s", cfg.DatabaseURL)

	// 4. 装配 stores
	serverStore := store.NewServerStore(database)
	projectStore := store.NewProjectStore(database)
	taskStore := store.NewTaskStore(database)
	apiKeyStore := store.NewApiKeyStore(database)
	sessionStore := store.NewSessionStore(database)
	eventStore := store.NewEventStore(database)
	snapshotStore := store.NewSnapshotStore(database)

	// 5. 装配 sshPool（AuthResolver 由 ServerService 实现）
	serverService := service.NewServerService(serverStore, masterKey)
	sshPool := ssh.NewPool(serverService)

	// 6. 装配 eventbus + runner
	bus := eventbus.New(eventStore, snapshotStore)
	r := runner.New(sshPool, bus)

	// 7. 装配 services
	projectService := service.NewProjectService(projectStore)
	taskService := service.NewTaskService(taskStore)
	apiKeyService := service.NewApiKeyService(apiKeyStore, masterKey)
	fileService := service.NewFileService(sshPool)
	sessionService := service.NewSessionService(sessionStore, bus, r, cfg.MockCli)
	snapshotService := service.NewSnapshotService(snapshotStore, sshPool)

	// 8. 装配 handlers
	deps := httptransport.Dependencies{
		Servers:   httptransport.NewServerHandler(serverService),
		Projects:  httptransport.NewProjectHandler(projectService),
		Tasks:     httptransport.NewTaskHandler(taskService),
		ApiKeys:   httptransport.NewApiKeyHandler(apiKeyService),
		Files:     httptransport.NewFileHandler(fileService),
		Sessions:  httptransport.NewSessionHandler(sessionService),
		Snapshots: httptransport.NewSnapshotHandler(snapshotService),
		WS:        ws.NewHandler(sessionService),
	}
	handler := httptransport.NewRouter(deps)

	// 9. 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 0, // WS 长连接不设写超时
		IdleTimeout:  120 * time.Second,
	}

	// 10. 优雅退出
	errCh := make(chan error, 1)
	go func() {
		log.Printf("HeyCode 后端监听 %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Printf("收到信号 %v，开始优雅关闭…", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Printf("已关闭")
	return nil
}
