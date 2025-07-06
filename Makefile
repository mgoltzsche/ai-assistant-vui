CLI_IMAGE?=ai-assistant-vui
SERVER_IMAGE?=ai-assistant-vui-server

INPUT_DEVICE=KLIM Talk
OUTPUT_DEVICE=ALC1220 Analog

all: help

##@ Build

build-vui: ## Build the CLI container.
	docker build --rm -t $(CLI_IMAGE) .

build-server: ## Build the web API server container.
	docker build --rm -t $(SERVER_IMAGE) -f Dockerfile-server .

build-webui: ui/node_modules ## (Re)build the web UI only.
	cd ui && npm run build

ui/node_modules:
	cd ui && npm install

run-localai: ## Run the LocalAI container.
	mkdir -p data/models data/backends
	docker run -ti --rm --network=host --privileged -v "`pwd`/data/models:/models" -v "`pwd`/data/backends:/backends" localai/localai:v3.1.1-vulkan

run-vui: build-vui ## Run the command line VUI.
	docker run --rm --privileged --network=host -v /var/run/docker.sock:/var/run/docker.sock $(CLI_IMAGE) --input-device="$(INPUT_DEVICE)" --output-device="$(OUTPUT_DEVICE)"

run-server: build-server ui/dist ## Run the VUI web API server.
	docker run --rm --network=host -v /var/run/docker.sock:/var/run/docker.sock -v "`pwd`/ui/dist:/var/lib/ai-assistant-vui/ui" $(SERVER_IMAGE) --tls

ui/dist: build-server
	rm -rf ui/dist && \
	CID=`docker create $(SERVER_IMAGE)` && \
	docker cp $$CID:/var/lib/ai-assistant-vui/ui ui/dist && \
	docker rm -f $$CID

clean: ## Delete build artifacts from repo dir.
	rm -rf ui/dist || true
	rm -rf ui/node_modules || true

##@ Development

render-diagrams: ## Render PNGs from PlantUML diagrams.
	docker run --rm -v "`pwd`/docs:/data" plantuml/plantuml:1.2025 *.puml

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
