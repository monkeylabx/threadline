COMPOSE ?= docker compose
PROJECT ?= threadline-dev
ARCHIVE ?= threadline-dev-images.tar
CHECKSUM ?= $(ARCHIVE).sha256
PLATFORM_FILE ?= $(ARCHIVE).platform
DOCKER_VERSION ?= 29.6.2
COMPOSE_VERSION ?= 5.4.0
PLATFORM ?= linux/$(shell docker version --format '{{.Server.Arch}}' 2>/dev/null)

CORE_IMAGES := \
	postgres:16.4-alpine@sha256:5660c2cbfea50c7a9127d17dc4e48543eedd3d7a41a595a2dfa572471e37e64c \
	nats:2.10.20-alpine@sha256:2c19fe5c950ed2ea9e8c9d1f16d89d2f4a2595e51fa0d3ac37d8c537be30aec9 \
	redis:7.4.0-alpine@sha256:c35af3bbcef51a62c8bae5a9a563c6f1b60d7ebaea4cb5a3ccbcc157580ae098 \
	minio/minio:RELEASE.2024-07-16T23-46-41Z@sha256:77ff9f7e12549d269990b167fa21da010fa8b0beb40d6064569b8887e37c456b \
	otel/opentelemetry-collector-contrib:0.107.0@sha256:b65527791431d76d058b2813748a3f4a8912540d7b23beac2f6b4e02c872f5b7 \
	prom/prometheus:v2.54.1@sha256:f6639335d34a77d9d9db382b92eeb7fc00934be8eae81dbc03b31cfe90411a94 \
	jaegertracing/jaeger:2.20.0@sha256:46a886260e04002d8f45e213fc39063fa11a50446048fdaa64786fc0840cb9f8

SUPPORT_IMAGES := busybox:1.36.1@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
IMAGES := $(CORE_IMAGES) $(SUPPORT_IMAGES)

ARGS := --project-name $(PROJECT) --file $(CURDIR)/compose.yaml

.PHONY: check check-images check-tools config destroy down images load-images logs recreate save-images status up verify-lock

check-tools:
	@command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }

check: check-tools
	@actual="$$(docker version --format '{{.Server.Version}}')"; test "$$actual" = "$(DOCKER_VERSION)" || { echo "Docker Engine $(DOCKER_VERSION) required; found $$actual" >&2; exit 1; }
	@actual="$$( $(COMPOSE) version --short )"; actual="$${actual#v}"; test "$$actual" = "$(COMPOSE_VERSION)" || { echo "Docker Compose $(COMPOSE_VERSION) required; found $$actual" >&2; exit 1; }
	@docker info --format '{{json .DriverStatus}}' | grep -Fq 'io.containerd.snapshotter.v1' || { echo "Docker containerd image store is required" >&2; exit 1; }
	@printf '%s\n' "$(PLATFORM)" | grep -Eq '^linux/(amd64|arm64)$$' || { echo "PLATFORM must be linux/amd64 or linux/arm64" >&2; exit 1; }

verify-lock:
	@sh "$(CURDIR)/../../docs/development/verify-dev-stack-image-locks.sh" exact "$(CURDIR)/compose.yaml" $(IMAGES)
	@sh "$(CURDIR)/../../docs/development/verify-dev-stack-image-locks.sh" common "$(CURDIR)/compose.yaml" "$(CURDIR)/../kind/stack.yaml" $(CORE_IMAGES)

check-images: check verify-lock
	@for image in $(IMAGES); do \
		digest="$${image##*@}"; \
		docker image inspect "$$image" >/dev/null 2>&1 || { \
			echo "missing local image: $$image" >&2; \
			echo "run 'make -C deploy/compose images' while connected, or load-images" >&2; \
			exit 1; \
		}; \
		docker image inspect --format '{{join .RepoDigests "\n"}}' "$$image" | grep -Fq "@$$digest" || { echo "RepoDigest mismatch: $$image" >&2; exit 1; }; \
	done

config: check verify-lock
	@$(COMPOSE) $(ARGS) config --quiet

images: check verify-lock
	@for image in $(IMAGES); do docker pull --platform "$(PLATFORM)" "$$image" || exit 1; done

save-images: check-images
	@docker image save --platform "$(PLATFORM)" --output "$(ARCHIVE)" $(IMAGES)
	@if command -v sha256sum >/dev/null 2>&1; then sha256sum "$(ARCHIVE)" | awk '{print $$1}' > "$(CHECKSUM)"; else shasum -a 256 "$(ARCHIVE)" | awk '{print $$1}' > "$(CHECKSUM)"; fi
	@printf '%s\n' "$(PLATFORM)" > "$(PLATFORM_FILE)"

load-images: check
	@test -f "$(ARCHIVE)" || { echo "archive not found: $(ARCHIVE)" >&2; exit 1; }
	@test -f "$(CHECKSUM)" || { echo "checksum not found: $(CHECKSUM)" >&2; exit 1; }
	@test -f "$(PLATFORM_FILE)" || { echo "platform metadata not found: $(PLATFORM_FILE)" >&2; exit 1; }
	@test "$$(tr -d '[:space:]' < "$(PLATFORM_FILE)")" = "$(PLATFORM)" || { echo "archive platform does not match $(PLATFORM)" >&2; exit 1; }
	@expected="$$(tr -d '[:space:]' < "$(CHECKSUM)")"; if command -v sha256sum >/dev/null 2>&1; then actual="$$(sha256sum "$(ARCHIVE)" | awk '{print $$1}')"; else actual="$$(shasum -a 256 "$(ARCHIVE)" | awk '{print $$1}')"; fi; test "$$actual" = "$$expected" || { echo "archive SHA-256 mismatch" >&2; exit 1; }
	@docker image load --platform "$(PLATFORM)" --input "$(ARCHIVE)"
	@$(MAKE) check-images

up: check-images config
	@$(COMPOSE) $(ARGS) up --detach --wait --pull never

down: check-tools
	@$(COMPOSE) $(ARGS) down --remove-orphans

destroy: check-tools
	@$(COMPOSE) $(ARGS) down --volumes --remove-orphans

recreate:
	@$(MAKE) destroy
	@$(MAKE) up

status: check-tools
	@$(COMPOSE) $(ARGS) ps

logs: check-tools
	@$(COMPOSE) $(ARGS) logs --follow --tail 100
