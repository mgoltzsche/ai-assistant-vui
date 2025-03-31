IMAGE?=ai-assistant-vui

INPUT_DEVICE=KLIM Talk
OUTPUT_DEVICE=ALC1220 Analog


all: build

build:
	docker build --rm -t $(IMAGE) .

run-localai:
	docker run -ti --rm --network=host --privileged -v `pwd`/models:/build/models localai/localai:v2.27.0-vulkan-ffmpeg-core

run-vui: build
	mkdir -p output
	docker run --rm --privileged --network=host -v /var/run/docker.sock:/var/run/docker.sock $(IMAGE) --input-device="$(INPUT_DEVICE)" --output-device="$(OUTPUT_DEVICE)"
