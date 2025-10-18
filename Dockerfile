ARG ONNXRUNTIME_VERSION=1.23.1

# Alpine build doesn't work since execinfo.h is not available on alpine/musl libc and due to some other issue
# Though, in the future there might be an alpine package since one is already being built in the edge version.
#FROM golang:1.23-alpine3.21 AS onnxruntime-alpine
#RUN apk add --update --no-cache git bash python3 cmake make gcc g++ pkgconf patch musl-dev linux-headers portaudio-dev
#ARG ONNXRUNTIME_VERSION
#RUN git clone --branch v$ONNXRUNTIME_VERSION --depth 1 --recurse-submodules https://github.com/microsoft/onnxruntime.git /onnxruntime
#WORKDIR /onnxruntime
#RUN sed -i '/#include <execinfo.h>/d' onnxruntime/core/platform/posix/stacktrace.cc
#RUN ./build.sh --allow_running_as_root --skip_submodule_sync --config Release --build_shared_lib --build_wheel --update --build --parallel --cmake_extra_defines


FROM debian:12-slim AS onnxruntime
RUN apt-get update && apt-get upgrade -y
RUN apt-get install -y git curl python3 build-essential g++ portaudio19-dev
# when builing with --build_wheel, install: python3-dev python3-packaging python3-setuptools python3-numpy-dev python3-wheel
ARG ONNXRUNTIME_VERSION
RUN git clone --branch v$ONNXRUNTIME_VERSION --depth 1 --recurse-submodules https://github.com/microsoft/onnxruntime.git /onnxruntime
WORKDIR /onnxruntime
# Replace cmake version to align with the version on the main branch - release/tag contains older version that doesn't work with the code
RUN sed -Ei "s/CMAKE_VERSION=.+/CMAKE_VERSION=3.28.0/" ./dockerfiles/scripts/install_cmake.sh
RUN ./dockerfiles/scripts/install_cmake.sh
RUN ./build.sh --allow_running_as_root --skip_submodule_sync --config Release --build_shared_lib --update --build --parallel --cmake_extra_defines ONNXRUNTIME_VERSION=$(cat ./VERSION_NUMBER)


FROM golang:1.24-bookworm AS vui
RUN apt-get update && apt-get upgrade -y
RUN apt-get install -y portaudio19-dev
COPY go.mod go.sum /build/
WORKDIR /build
RUN go mod download
ARG ONNXRUNTIME_VERSION
COPY --from=onnxruntime /onnxruntime/build/Linux/Release/libonnxruntime.so* /usr/local/lib/onnxruntime/
COPY --from=onnxruntime /onnxruntime/include /usr/local/include/onnxruntime-$ONNXRUNTIME_VERSION/include
COPY cmd/ai-assistant-vui /build/cmd/ai-assistant-vui
COPY internal /build/internal
COPY pkg /build/pkg
ENV CGO_ENABLED=1
ENV C_INCLUDE_PATH="/usr/local/include/onnxruntime-$ONNXRUNTIME_VERSION/include/onnxruntime/core/session" \
	LIBRARY_PATH="/usr/local/lib/onnxruntime" \
	LD_RUN_PATH="/usr/local/lib/onnxruntime"
RUN go build -o ai-assistant-vui ./cmd/ai-assistant-vui


FROM debian:12-slim
RUN set -ex; \
	apt-get update && apt-get upgrade -y; \
	apt-get install -y libportaudiocpp0 curl
ARG ONNXRUNTIME_VERSION
COPY --from=onnxruntime /onnxruntime/build/Linux/Release/libonnxruntime.so* /usr/local/lib/onnxruntime/
COPY --from=onnxruntime /onnxruntime/include /usr/local/include/onnxruntime-$ONNXRUNTIME_VERSION/include
#RUN set -eux; \
#	mkdir /models; \
#	wget -qO /models/silero-vad-en_v5.onnx https://models.silero.ai/models/en/en_v5.onnx
ARG SILERO_VAD_VERSION=v5.1.2
RUN set -eux; \
	mkdir /models; \
	curl -fsSL https://github.com/snakers4/silero-vad/raw/refs/tags/$SILERO_VAD_VERSION/src/silero_vad/data/silero_vad.onnx > /models/silero_vad.onnx
COPY --from=vui /build/ai-assistant-vui /
COPY config.yaml /etc/ai-assistant-vui/config.yaml
ENV VUI_LOG_LEVEL=DEBUG
ENTRYPOINT ["/ai-assistant-vui"]
