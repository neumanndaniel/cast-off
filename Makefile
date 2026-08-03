# SPDX-License-Identifier: Apache-2.0
# Copyright Daniel Neumann

BINARY_NAME ?= cast-off
BIN_DIR ?= bin
CMD_PATH ?= ./cmd/cast-off
GOOS ?= linux
GOARCH ?= amd64
REGISTRY ?= quay.io
REPOSITORY ?= neumanndaniel
IMAGE_NAME ?= cast-off
IMAGE_TAG ?= dev
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64
CHART_DIR ?= charts/cast-off
CHART_NAME ?= cast-off
CHART_DIST_DIR ?= dist
CHART_OCI_REPO ?= oci://$(REGISTRY)/$(REPOSITORY)
CHART_VERSION_CURRENT := $(shell awk '/^version:/ {print $$2}' $(CHART_DIR)/Chart.yaml)
RELEASE_VERSION ?= $(CHART_VERSION_CURRENT)
CHART_PACKAGE_FILE := $(CHART_DIST_DIR)/$(CHART_NAME)-$(RELEASE_VERSION).tgz
RELEASE_NOTES_FILE := $(CHART_DIST_DIR)/release-notes-$(RELEASE_VERSION).md
RELEASE_BUNDLE_DIR := $(CHART_DIST_DIR)/release-$(RELEASE_VERSION)

.PHONY: help tidy test govulncheck build build-linux build-linux-amd64 build-linux-arm64 build-linux-all docker-build docker-buildx docker-push docker-release-multi chart-lint chart-version chart-package chart-verify-package chart-push chart-release-local chart-release release-notes release-bundle run clean

help:
	@echo "Available targets:"
	@echo "  tidy        - Run go mod tidy"
	@echo "  test        - Run go test ./..."
	@echo "  govulncheck - Run govulncheck via go run"
	@echo "  build       - Build local host binary to $(BIN_DIR)/$(BINARY_NAME)"
	@echo "  build-linux - Build Linux binary to $(BIN_DIR)/$(BINARY_NAME)-linux-$(GOARCH)"
	@echo "  build-linux-amd64 - Build Linux amd64 binary"
	@echo "  build-linux-arm64 - Build Linux arm64 binary"
	@echo "  build-linux-all - Build Linux binaries for amd64 and arm64"
	@echo "  docker-build - Build multi-arch image for $(DOCKER_PLATFORMS)"
	@echo "  docker-buildx - Alias of docker-build"
	@echo "  docker-push - Push multi-arch image for $(DOCKER_PLATFORMS)"
	@echo "  docker-release-multi - Build and push multi-arch image"
	@echo "  chart-lint - Lint and template-check Helm chart"
	@echo "  chart-version - Set chart version and appVersion to RELEASE_VERSION"
	@echo "  chart-package - Package Helm chart to $(CHART_DIST_DIR)/"
	@echo "  chart-verify-package - Verify packaged chart exists"
	@echo "  chart-push - Push packaged chart to $(CHART_OCI_REPO)"
	@echo "  chart-release-local - Run local chart release flow without push"
	@echo "  chart-release - Run full local chart release flow including push"
	@echo "  release-notes - Extract release notes for RELEASE_VERSION"
	@echo "  release-bundle - Collect chart package and notes into release folder"
	@echo "  run         - Run the service locally"
	@echo "  clean       - Remove generated binaries"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

tidy:
	go mod tidy

test:
	nocorrect go test ./...

govulncheck:
	nocorrect go run golang.org/x/vuln/cmd/govulncheck@latest ./...

build: $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

build-linux: $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/$(BINARY_NAME)-linux-$(GOARCH) $(CMD_PATH)

build-linux-amd64:
	$(MAKE) build-linux GOARCH=amd64

build-linux-arm64:
	$(MAKE) build-linux GOARCH=arm64

build-linux-all: build-linux-amd64 build-linux-arm64

docker-build:
	docker buildx build --platform=$(DOCKER_PLATFORMS) -t $(REGISTRY)/$(REPOSITORY)/$(IMAGE_NAME):$(IMAGE_TAG) .

docker-buildx:
	$(MAKE) docker-build

docker-push:
	docker buildx build --platform=$(DOCKER_PLATFORMS) -t $(REGISTRY)/$(REPOSITORY)/$(IMAGE_NAME):$(IMAGE_TAG) --push .

docker-release-multi: docker-push

chart-lint:
	helm lint $(CHART_DIR)
	helm template $(CHART_NAME) $(CHART_DIR) > /dev/null

chart-version:
	@awk -v v="$(RELEASE_VERSION)" 'BEGIN { updatedVersion=0; updatedAppVersion=0 } /^version:/ { print "version: " v; updatedVersion=1; next } /^appVersion:/ { print "appVersion: \"" v "\""; updatedAppVersion=1; next } { print } END { if (updatedVersion == 0) { print "version: " v } if (updatedAppVersion == 0) { print "appVersion: \"" v "\"" } }' $(CHART_DIR)/Chart.yaml > $(CHART_DIR)/Chart.yaml.tmp
	@mv $(CHART_DIR)/Chart.yaml.tmp $(CHART_DIR)/Chart.yaml

chart-package:
	@mkdir -p $(CHART_DIST_DIR)
	helm package $(CHART_DIR) --destination $(CHART_DIST_DIR) --version $(RELEASE_VERSION) --app-version $(RELEASE_VERSION)

chart-verify-package:
	@test -f $(CHART_PACKAGE_FILE) || (echo "missing chart artifact: $(CHART_PACKAGE_FILE)" && exit 1)

release-notes:
	@mkdir -p $(CHART_DIST_DIR)
	@awk -v v="$(RELEASE_VERSION)" 'BEGIN { in_section=0; pattern="^## \\[" v "\\]" } $$0 ~ pattern { in_section=1 } in_section && $$0 ~ /^## \[/ && $$0 !~ pattern { exit } in_section { print }' CHANGELOG.md > $(RELEASE_NOTES_FILE)
	@test -s $(RELEASE_NOTES_FILE) || (echo "no release notes found for version $(RELEASE_VERSION) in CHANGELOG.md" && exit 1)

release-bundle: chart-verify-package release-notes
	@mkdir -p $(RELEASE_BUNDLE_DIR)
	cp $(CHART_PACKAGE_FILE) $(RELEASE_BUNDLE_DIR)/
	cp $(RELEASE_NOTES_FILE) $(RELEASE_BUNDLE_DIR)/

chart-release-local: chart-lint chart-version chart-package chart-verify-package release-notes release-bundle

chart-push: chart-release-local
	helm push $(CHART_PACKAGE_FILE) $(CHART_OCI_REPO)

chart-release: chart-push

run:
	go run $(CMD_PATH)

clean:
	rm -rf $(BIN_DIR) $(CHART_DIST_DIR)
