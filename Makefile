.DEFAULT_GOAL := help

# Переменные
BINARY_NAME := updater
BINARY_PATH := ./cmd/updater/main.go
CONFIG_PATH := config.yaml
BUILD_DIR := ./build

# Go настройки
GOPROXY := direct

.PHONY: help build run test test-race test-verbose lint fmt vet check tidy mocks install

help: ## Показать справку по командам
	@echo "Доступные команды:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Примеры:"
	@echo "  make build       # Собрать бинарник"
	@echo "  make run         # Запустить updater"
	@echo "  make test        # Запустить тесты"
	@echo "  make check       # Полная проверка"

build: ## Собрать бинарник в build/updater
	@echo "Сборка $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	GOPROXY=$(GOPROXY) go build -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	@echo "Готово: $(BUILD_DIR)/$(BINARY_NAME)"

run: build ## Собрать и запустить updater
	@echo "Запуск updater..."
	@$(BUILD_DIR)/$(BINARY_NAME)

dev: ## Запуст updater без сборки (go run)
	@echo "Запуск в dev-режиме..."
	GOPROXY=$(GOPROXY) go run $(BINARY_PATH)

test: ## Запустить тесты
	@echo "Запуск тестов..."
	go test ./...

test-verbose: ## Запустить тесты с подробным выводом
	@echo "Запуск тестов (verbose)..."
	go test -v ./...

test-race: ## Запустить тесты с race detector
	@echo "Запуск тестов с race detector..."
	go test -race ./...

test-cover: ## Запустить тесты с coverage
	@echo "Запуск тестов с coverage..."
	go test -cover ./...

test-cover-html: ## Запустить тесты и открыть coverage report
	@echo "Генерация coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Отчёт: coverage.html"

lint: ## Запустить линтеры (go vet)
	@echo "Запуск go vet..."
	go vet ./...

fmt: ## Отформатировать код
	@echo "Форматирование..."
	gofmt -w .

vet: lint ## Алиас для lint

check: fmt vet test build ## Полная проверка: форматирование, линтер, тесты, сборка
	@echo ""
	@echo "✅ Все проверки пройдены!"

tidy: ## Обновить go.mod и go.sum
	@echo "Tidy..."
	GOPROXY=$(GOPROXY) go mod tidy

mocks: ## Сгенерировать моки (mockery)
	@echo "Генерация моков..."
	go tool mockery

install: ## Установить инструменты (mockery)
	@echo "Установка инструментов..."
	go install github.com/vektra/mockery/v3@latest

.DEFAULT:
	@echo "Неизвестная команда: $@"
	@echo ""
	@$(MAKE) help
