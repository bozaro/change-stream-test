#!/bin/bash -ex

# Build runtime environment docker image (with MongoDB)
docker build -t change-stream-test-runtime:latest docker/

# Compile application
docker run --volume "$(pwd):/app" --workdir /app golang:1.14 go build .

# Run testing application
docker run --volume "$(pwd):/app" change-stream-test-runtime:latest /app/change-stream-test
