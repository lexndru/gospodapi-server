#!/usr/bin/make

.PHONY: all clean deps install test cloud cloud-psql cloud-mysql local demo cover heatmap dist resources

CWD=$(shell pwd)

GO_BUILD=go build
GO_CLEAN=go clean
GO_TEST=go test
GO_MOD=go mod
GO_TOOL=go tool

BUILD_DIR=build
BUILD_OS=$(shell uname)
BUILD_ARCH=$(shell uname -i)
BUILD_COMMIT=$(shell git log -1 --date=short --format="%h_%ad")

BIN_VERSION=0.5.2
BIN_NAME=gospodapi
BIN_FLAGS=-X main.VERSION=$(BIN_VERSION) -X main.BUILD=$(BUILD_COMMIT) -X main.OSARCH=$(BUILD_OS)/$(BUILD_ARCH)

INSTALL_PATH=/opt/gospodapi

all: clean deps test demo

define INSTALLER
#!/bin/sh

adduser --system --group --disabled-password --no-create-home $(BIN_NAME)

mkdir -p $(INSTALL_PATH)

cp -v LICENSE $(INSTALL_PATH)/LICENSE
cp -v env.conf $(INSTALL_PATH)/env.conf
cp -v $(BIN_NAME) $(INSTALL_PATH)/$(BIN_NAME)
cp -v $(BIN_NAME).service /lib/systemd/system/$(BIN_NAME).service

chown $(BIN_NAME):$(BIN_NAME) -R $(INSTALL_PATH)

echo ""
echo "install done, don't forget to enable:"
echo "  systemctl daemon-reload && systemctl enable $(BIN_NAME).service"
echo ""
echo "this folder can now be removed"
endef

define UNIT_FILE
[Unit]
Description=$(BIN_NAME)
After=network.target

[Service]
User=$(BIN_NAME)
EnvironmentFile=-$(INSTALL_PATH)/env.conf
ExecStart=$(INSTALL_PATH)/$(BIN_NAME)
Restart=always
RestartSec=10
StandardOutput=syslog
StandardError=syslog
WorkingDirectory=$(INSTALL_PATH)
SyslogIdentifier=$(BIN_NAME)

[Install]
WantedBy=multi-user.target
Alias=$(BIN_NAME).service
endef

export UNIT_FILE
export INSTALLER

dist:
	mkdir -p dist
	echo "$$UNIT_FILE" > dist/$(BIN_NAME).service
	echo "$$INSTALLER" > dist/install
	cp -v build/* dist/
	cp -v LICENSE dist/
	tar -czvf dist-$(BIN_VERSION).tar.gz dist/*

resources/%.zip:
	cd resources/$* && zip $*.zip reg_actors.json reg_labels.json reg_transactions.json
	mv resources/$*/$*.zip $@

cloud: cloud-psql
	@echo "\n(default database in cloud is psql)"

cloud-psql:
	$(GO_BUILD) -ldflags "$(BIN_FLAGS) -X main.DRIVER=psql -X main.LICENSE=cloud" -o $(BUILD_DIR)/$(BIN_NAME) -v -x
	echo 'DB_DSN="host=localhost user=gospodapi password=gospodapi dbname=postgres port=5432 sslmode=disable TimeZone=UTC"' > $(BUILD_DIR)/env.conf

cloud-mysql:
	$(GO_BUILD) -ldflags "$(BIN_FLAGS) -X main.DRIVER=mysql -X main.LICENSE=cloud" -o $(BUILD_DIR)/$(BIN_NAME) -v -x
	echo 'DB_DSN="gospodapi:password@tcp(localhost:3306)/mariadb?charset=utf8mb4&parseTime=True"' > $(BUILD_DIR)/env.conf

local:
	$(GO_BUILD) -ldflags "$(BIN_FLAGS) -X main.DRIVER=sqlite -X main.LICENSE=local" -o $(BUILD_DIR)/$(BIN_NAME) -v -x
	echo 'DB_DSN="sqlite.db"' > $(BUILD_DIR)/env.conf

clean:
	$(GO_CLEAN) -x -v
	rm -f $(BUILD_DIR)/$(BIN_NAME) 2> /dev/null

deps:
	$(GO_MOD) tidy

test:
	$(GO_TEST) -v -cover ./...

demo:
	$(GO_BUILD) -ldflags "$(BIN_FLAGS)" -o $(BUILD_DIR)/$(BIN_NAME) -v -x

demo-windows:
	GOOS=windows $(GO_BUILD) -ldflags "$(BIN_FLAGS)" -o $(BUILD_DIR)/$(BIN_NAME) -v -x

cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

heatmap:
	go test -covermode=count -coverprofile=heatmap.out ./...
	go tool cover -html=heatmap.out