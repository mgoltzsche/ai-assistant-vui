#!/bin/sh

# Makes podman serve the docker socket, if there is no socket already
# Usage: with-docker.sh {COMMAND}

if [ ! -S /var/run/docker.sock ]; then
	echo Starting podman since /var/run/docker.sock is not a socket
	export DOCKER_HOST=unix:///tmp/podman.sock
	podman system service --time=0 "$DOCKER_HOST" &
	sleep 3
fi

exec "$@"
