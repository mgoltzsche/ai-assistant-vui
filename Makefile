CLI_IMAGE?=ai-assistant-vui
SERVER_IMAGE?=ai-assistant-vui-server

INPUT_DEVICE=KLIM Talk
OUTPUT_DEVICE=ALC1220 Analog

BIN_DIR=$(PWD)/build/bin
PROTOC_VERSION=34.1
PROTOC_GEN_GO=$(BIN_DIR)/protoc-gen-go
PROTOC_GEN_GO_VERSION=v1.36.11


all: help

##@ Build

build-vui: generate ## Build the CLI container.
	docker build --rm -t $(CLI_IMAGE) .

build-server: generate ## Build the web API server container.
	docker build --rm -t $(SERVER_IMAGE) -f Dockerfile-server .

build-webui: ui/node_modules ## (Re)build the web UI only.
	cd ui && npm run generate && npm run build

ui/node_modules:
	cd ui && npm install

run-localai: ## Run the LocalAI container.
	mkdir -p data/models data/backends
	docker run -ti --rm --network=host --privileged -v "`pwd`/data/models:/models" -v "`pwd`/data/backends:/backends" localai/localai:v4.1.2-gpu-vulkan

run-vui: build-vui ## Run the command line VUI.
	docker run --rm --privileged --network=host -v "`pwd`/data/memory:/data/memory" $(CLI_IMAGE) --input-device="$(INPUT_DEVICE)" --output-device="$(OUTPUT_DEVICE)"

run-server: build-server ui/dist ## Run the VUI web API server.
	$(eval DOCKEROPTS = $(if $(wildcard /var/run/docker.sock),-v /var/run/docker.sock:/var/run/docker.sock,--privileged))
	docker run --rm --network=host $(DOCKEROPTS) -v "`pwd`/data/memory:/data/memory" -v "`pwd`/ui/dist:/var/lib/ai-assistant-vui/ui" $(SERVER_IMAGE) --tls

generate: protoc protoc-gen-go ## Generate Go code from protobuf
	export PATH="$(BIN_DIR):$$PATH"; \
	go generate ./...

ui/dist: build-server
	rm -rf ui/dist && \
	CID=`docker create $(SERVER_IMAGE)` && \
	docker cp $$CID:/var/lib/ai-assistant-vui/ui ui/dist && \
	docker rm -f $$CID

clean: ## Delete build artifacts from repo dir.
	rm -rf internal/generated/api
	rm -rf ui/dist || true
	rm -rf ui/node_modules || true

##@ Development

render-diagrams: ## Render PNGs from PlantUML diagrams.
	docker run --rm -v "`pwd`/docs:/data" plantuml/plantuml:1.2025 *.puml

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


protoc: OS=$(shell uname | tr '[:upper:]' '[:lower:]')
protoc: ARCH=$(shell uname -m)
protoc:
	@[ -f $(BIN_DIR)/protoc ] || { \
	echo 'Downloading protoc $(PROTOC_VERSION)'; \
	mkdir -p $(BIN_DIR); \
	TMP_DIR=`mktemp -d --suffix=-aivui-proto`; \
	trap 'rm -rf $$TMP_DIR' EXIT; \
	curl -fsSL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(OS)-$(ARCH).zip > $$TMP_DIR/protoc.zip; \
	unzip -p $$TMP_DIR/protoc.zip bin/protoc > $(BIN_DIR)/protoc; \
	chmod +x $(BIN_DIR)/protoc; }

protoc-gen-go:
	$(call go-install-tool,$(PROTOC_GEN_GO),google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION))

# go-install-tool will 'go install' any package $2 and install it to $1.
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
mkdir -p $(BIN_DIR); \
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(BIN_DIR) go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
