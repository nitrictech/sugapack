.PHONY: build image test clean infra infra-stop dry-run dry-run-private

# Internal registry hostname (as seen by buildkitd inside the Docker network)
IMAGE       := sugapack-registry:5000/sugapack:local
BUILDKITD   := sugapack-buildkitd
REGISTRY_C  := sugapack-registry
NETWORK     := sugapack-net
SOCK        := /tmp/sugapack-buildkit/buildkitd.sock
ADDR        := unix://$(SOCK)
TEST_CONFIG ?= test.json

# ── Local dev ────────────────────────────────────────────────

build:
	go build -o sugapack .

test:
	go test ./... -v

# ── Docker / BuildKit infra ──────────────────────────────────

# Start local registry + buildkitd on a shared network
infra:
	@docker network inspect $(NETWORK) >/dev/null 2>&1 || \
		docker network create $(NETWORK)
	@if ! docker inspect $(REGISTRY_C) >/dev/null 2>&1; then \
		echo "Starting registry..."; \
		docker run -d --name $(REGISTRY_C) --network $(NETWORK) registry:2; \
	elif [ "$$(docker inspect -f '{{.State.Running}}' $(REGISTRY_C))" != "true" ]; then \
		docker start $(REGISTRY_C); \
	else \
		echo "registry already running"; \
	fi
	@mkdir -p $(dir $(SOCK))
	@if ! docker inspect $(BUILDKITD) >/dev/null 2>&1; then \
		echo "Starting buildkitd..."; \
		docker run -d --name $(BUILDKITD) --privileged \
			--network $(NETWORK) \
			-v $(dir $(SOCK)):/run/buildkit \
			-v $(CURDIR)/buildkitd.toml:/etc/buildkit/buildkitd.toml:ro \
			moby/buildkit:latest \
			--addr unix:///run/buildkit/buildkitd.sock \
			--group $(shell id -g); \
	elif [ "$$(docker inspect -f '{{.State.Running}}' $(BUILDKITD))" != "true" ]; then \
		docker start $(BUILDKITD); \
	else \
		echo "buildkitd already running"; \
	fi
	@echo "Waiting for buildkitd..."
	@for i in 1 2 3 4 5; do \
		buildctl --addr $(ADDR) debug workers >/dev/null 2>&1 && break; \
		sleep 1; \
	done

infra-stop:
	docker rm -f $(BUILDKITD) $(REGISTRY_C) 2>/dev/null || true
	docker network rm $(NETWORK) 2>/dev/null || true
	rm -rf $(dir $(SOCK))

# Build and push the single image (frontend + railpack CLI) to local registry
image: infra
	buildctl --addr $(ADDR) build \
		--frontend dockerfile.v0 \
		--local context=. \
		--local dockerfile=. \
		--output type=image,name=$(IMAGE),push=true,registry.insecure=true

# ── Dry-run ──────────────────────────────────────────────────
#
#   make dry-run                                            # public repo, uses test.json
#   make dry-run TEST_CONFIG=myapp.json                     # public repo, custom config
#   GIT_AUTH_TOKEN=ghp_xxx make dry-run-private              # private repo
#   GIT_AUTH_TOKEN=ghp_xxx make dry-run-private TEST_CONFIG=private.json

dry-run: image
	buildctl --addr $(ADDR) build \
		--frontend gateway.v0 \
		--opt source=$(IMAGE) \
		--local dockerfile=. \
		--opt filename=$(TEST_CONFIG) \
		--output type=docker,name=sugapack-test-output | docker load

dry-run-private: image
	buildctl --addr $(ADDR) build \
		--frontend gateway.v0 \
		--opt source=$(IMAGE) \
		--local dockerfile=. \
		--opt filename=$(TEST_CONFIG) \
		--secret id=GIT_AUTH_TOKEN,env=GIT_AUTH_TOKEN \
		--output type=docker,name=sugapack-test-output | docker load

# ── Cleanup ──────────────────────────────────────────────────

clean: infra-stop
	rm -f sugapack
