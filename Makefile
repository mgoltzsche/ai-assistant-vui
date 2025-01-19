IMAGE?=ai-assistant-vui

INPUT_DEVICE=KLIM Talk
OUTPUT_DEVICE=ALC1220 Analog


all: build

build:
	docker build --rm -t $(IMAGE) .

run-localai:
	docker run -ti --rm --network=host --privileged -v `pwd`/models:/build/models localai/localai:v2.25.0-vulkan-ffmpeg-core
	# To run with GPU support: docker run -ti --rm --network=host --privileged --device=/dev/kfd --device=/dev/dri --security-opt seccomp=unconfined --group-add video -v `pwd`/models:/build/models $(IMAGE)

run-vui: build
	mkdir -p output
	docker run --rm --privileged --network=host $(IMAGE) --input-device="$(INPUT_DEVICE)" --output-device="$(OUTPUT_DEVICE)"
