# HeyCode 后端 Makefile
APP := heycode-backend
PKG := ./cmd/heycode-backend

.PHONY: build run test vet tidy clean cross-build-linux mock

## build: 编译当前平台二进制
build:
	go build -o $(APP) $(PKG)

## run: 直接运行（默认 :8787, file:./dev.db）
run:
	go run $(PKG)

## mock: 启用 mock CLI 模式（无需真实 SSH/CLI）
mock:
	MOCK_CLI=1 go run $(PKG)

## test: 跑全部单元测试
test:
	go test ./...

## vet: 静态检查
vet:
	go vet ./...

## tidy: 整理依赖
tidy:
	go mod tidy

## clean: 清理本地构建产物与 dev 数据库
clean:
	rm -f $(APP) $(APP)-linux-amd64 dev.db dev.db-wal dev.db-shm

## cross-build-linux: 交叉编译 linux/amd64 二进制（无 CGO）
cross-build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(APP)-linux-amd64 $(PKG)
