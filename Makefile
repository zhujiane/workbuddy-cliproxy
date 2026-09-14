PLUGIN_ID ?= workbuddy
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
VERSION ?= 0.1.0
PLUGIN_DIR ?=

ifeq ($(GOOS),windows)
EXT := dll
else ifeq ($(GOOS),darwin)
EXT := dylib
else
EXT := so
endif

LIB := $(PLUGIN_ID).$(EXT)
LDFLAGS := -s -w -X main.pluginVersion=$(VERSION)

.PHONY: test build install clean

test:
	CGO_ENABLED=1 go test ./...

build:
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o $(LIB) .
	rm -f $(PLUGIN_ID).h

# PLUGIN_DIR is the host-side plugins directory mounted into CPA, e.g.
#   make install PLUGIN_DIR=../CLIProxyAPI/plugins RESTART_DOCKER=cli-proxy-api
install: build
	@test -n "$(PLUGIN_DIR)" || { echo "PLUGIN_DIR is required"; exit 1; }
	mkdir -p "$(PLUGIN_DIR)/$(GOOS)/$(GOARCH)"
	if [ -f "$(PLUGIN_DIR)/$(GOOS)/$(GOARCH)/$(LIB)" ]; then \
		cp -a "$(PLUGIN_DIR)/$(GOOS)/$(GOARCH)/$(LIB)" "$(PLUGIN_DIR)/$(GOOS)/$(GOARCH)/$(LIB).bak"; \
	fi
	cp "$(LIB)" "$(PLUGIN_DIR)/$(GOOS)/$(GOARCH)/$(LIB)"
	@if [ -n "$(RESTART_DOCKER)" ]; then docker restart "$(RESTART_DOCKER)"; fi

clean:
	rm -f $(PLUGIN_ID).so $(PLUGIN_ID).dylib $(PLUGIN_ID).dll $(PLUGIN_ID).h
	rm -rf dist
