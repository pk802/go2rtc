#!/bin/bash

# Build script for pk802/go2rtc Docker images
# Usage: ./build-docker.sh [tag] [dockerfile]

set -e

# Configuration
DOCKER_USERNAME="pk802"
IMAGE_NAME="go2rtc"
TAG="${1:-latest}"
DOCKERFILE="${2:-docker/Dockerfile}"

# Full image name
FULL_IMAGE_NAME="${DOCKER_USERNAME}/${IMAGE_NAME}:${TAG}"

echo "Building Docker image: ${FULL_IMAGE_NAME}"
echo "Using Dockerfile: ${DOCKERFILE}"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed. Please install Docker first."
    exit 1
fi

# Build the image
echo "Building image..."
docker build -f "${DOCKERFILE}" -t "${FULL_IMAGE_NAME}" .

echo "Build completed successfully!"
echo "Image: ${FULL_IMAGE_NAME}"

# Ask if user wants to push
read -p "Do you want to push this image to Docker Hub? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Pushing image to Docker Hub..."
    docker push "${FULL_IMAGE_NAME}"
    echo "Push completed!"
else
    echo "Image built locally. To push later, run:"
    echo "docker push ${FULL_IMAGE_NAME}"
fi

# Show available variants
echo ""
echo "Available build variants:"
echo "./build-docker.sh latest docker/Dockerfile           # Standard image"
echo "./build-docker.sh hardware docker/hardware.Dockerfile # Hardware acceleration"
echo "./build-docker.sh rockchip docker/rockchip.Dockerfile # Rockchip support"
echo ""
echo "To run the image:"
echo "docker run -d -p 1984:1984 -p 8554:8554 -v ./config:/config ${FULL_IMAGE_NAME}"
