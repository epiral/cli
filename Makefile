.PHONY: build lint fmt generate check clean web

# 完整构建（前端 + Go）
build: web
	go build -o bin/epiral ./cmd/epiral

# 仅构建 Go（使用已有的 dist）
build-go:
	go build -o bin/epiral ./cmd/epiral

# 构建前端并复制到 embed 目录
web:
	cd web && pnpm install --frozen-lockfile && pnpm build
	rm -rf internal/webserver/dist
	cp -r web/dist internal/webserver/dist

# 前端开发模式
dev:
	cd web && pnpm dev

# 代码生成
generate:
	buf lint
	buf generate

# 格式化
fmt:
	gofmt -s -w .
	goimports -w .

# Lint 检查
lint:
	golangci-lint run ./...

# 完整检查（提交前必跑）
check: fmt lint build
	@echo "✓ 所有检查通过"

# 清理
clean:
	rm -rf bin/
	rm -rf web/dist
	rm -rf web/node_modules
