BINARY_NAME=tgblobsync
BUILD_DIR=./bin
DOCKER_IMAGE=tgblobsync-builder

# Default target
all: build-local

# Build locally for the current OS
build-local:
	go build -ldflags "-s -w -X main.AppID=$(APP_ID) -X main.AppHash=$(APP_HASH)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/tgblobsync

# Build using Docker for all platforms
build-docker:
	@if [ -z "$(APP_ID)" ] || [ -z "$(APP_HASH)" ]; then \
		echo "Error: APP_ID and APP_HASH must be set"; \
		exit 1; \
	fi
	docker build \
		--build-arg APP_ID=$(APP_ID) \
		--build-arg APP_HASH=$(APP_HASH) \
		-t $(DOCKER_IMAGE) \
		-f build/Dockerfile .
	
	mkdir -p $(BUILD_DIR)
	# Create a container instance to copy files from
	$(eval CONTAINER_ID := $(shell docker create $(DOCKER_IMAGE)))
	docker cp $(CONTAINER_ID):/tgblobsync-linux-amd64 $(BUILD_DIR)/
	docker cp $(CONTAINER_ID):/tgblobsync-windows-amd64.exe $(BUILD_DIR)/
	docker cp $(CONTAINER_ID):/tgblobsync-darwin-amd64 $(BUILD_DIR)/
	docker cp $(CONTAINER_ID):/tgblobsync-darwin-arm64 $(BUILD_DIR)/
	docker rm $(CONTAINER_ID)
	@echo "Build artifacts placed in $(BUILD_DIR)"

clean:
	rm -rf $(BUILD_DIR)
