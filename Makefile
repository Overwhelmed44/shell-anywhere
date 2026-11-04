ifeq ($(OS),Windows_NT)
WAILS ?= $(shell go env GOPATH)/bin/wails.exe
else
WAILS ?= $(shell go env GOPATH)/bin/wails
endif
NPM ?= npm
WINDOWS_PLATFORM ?= windows/amd64
BUILD_TAGS ?=

.PHONY: help dev build installer frontend

help:
	@echo make dev       - run the desktop app with frontend hot reload
	@echo make build     - create the desktop app for this computer
	@echo make installer - create the Windows setup executable
	@echo make frontend  - run the browser-only frontend preview

dev:
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -Command "$$env:WAILS_DEV_ALLOWED_HOSTS = 'wails.localhost'; & '$(WAILS)' dev"
else
	WAILS_DEV_ALLOWED_HOSTS=wails.localhost "$(WAILS)" dev -tags "$(BUILD_TAGS)"
endif

build:
	"$(WAILS)" build -tags "$(BUILD_TAGS)"

installer:
	powershell -NoProfile -Command "if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) { Write-Error 'Install NSIS and add makensis.exe to PATH before building the installer'; exit 1 }"
	"$(WAILS)" build -platform $(WINDOWS_PLATFORM) -nsis -installscope user

frontend:
	$(NPM) --prefix frontend run dev
