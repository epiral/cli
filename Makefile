.PHONY: build lint fmt generate check clean

# 编译
build:
	go build -o bin/epiral ./cmd/epiral

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
