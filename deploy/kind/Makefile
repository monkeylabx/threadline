CLUSTER ?= threadline-dev
ARCHIVE ?= threadline-kind-images.tar
CHECKSUM ?= $(ARCHIVE).sha256
PLATFORM_FILE ?= $(ARCHIVE).platform
IDS ?= $(ARCHIVE).ids
IDS_CHECKSUM ?= $(IDS).sha256
OFFLINE_LOCK ?= $(CURDIR)/.threadline-loaded-image-ids
DOCKER_VERSION ?= 29.6.2
KIND_VERSION ?= v0.32.0
KUBECTL_VERSION ?= v1.36.2
KUBERNETES_VERSION ?= v1.36.1
PLATFORM ?= linux/$(shell docker version --format '{{.Server.Arch}}' 2>/dev/null)
NODE_IMAGE ?= kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5

CORE_IMAGES := \
	postgres:16.4-alpine@sha256:5660c2cbfea50c7a9127d17dc4e48543eedd3d7a41a595a2dfa572471e37e64c \
	nats:2.10.20-alpine@sha256:2c19fe5c950ed2ea9e8c9d1f16d89d2f4a2595e51fa0d3ac37d8c537be30aec9 \
	redis:7.4.0-alpine@sha256:c35af3bbcef51a62c8bae5a9a563c6f1b60d7ebaea4cb5a3ccbcc157580ae098 \
	minio/minio:RELEASE.2024-07-16T23-46-41Z@sha256:77ff9f7e12549d269990b167fa21da010fa8b0beb40d6064569b8887e37c456b \
	otel/opentelemetry-collector-contrib:0.107.0@sha256:b65527791431d76d058b2813748a3f4a8912540d7b23beac2f6b4e02c872f5b7 \
	prom/prometheus:v2.54.1@sha256:f6639335d34a77d9d9db382b92eeb7fc00934be8eae81dbc03b31cfe90411a94 \
	jaegertracing/jaeger:2.20.0@sha256:46a886260e04002d8f45e213fc39063fa11a50446048fdaa64786fc0840cb9f8

PROBE_IMAGE := busybox:1.36.1@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
IMAGES := $(CORE_IMAGES) $(PROBE_IMAGE)
TAG_CORE_IMAGES := $(foreach image,$(CORE_IMAGES),$(word 1,$(subst @, ,$(image))))
TAG_IMAGES := $(foreach image,$(IMAGES),$(word 1,$(subst @, ,$(image))))
TAG_NODE_IMAGE := $(word 1,$(subst @, ,$(NODE_IMAGE)))
TAG_PROBE_IMAGE := $(word 1,$(subst @, ,$(PROBE_IMAGE)))
ARCHIVE_HELPER := $(CURDIR)/../../docs/development/dev-stack-image-archive.sh

CONTEXT := kind-$(CLUSTER)

.PHONY: check check-images check-tools config destroy down images load-images recreate save-images status up verify-cluster verify-lock verify-network-policy

check-tools:
	@for command in docker kind kubectl; do \
		command -v "$$command" >/dev/null 2>&1 || { echo "$$command is required" >&2; exit 1; }; \
	done

check: check-tools
	@actual="$$(docker version --format '{{.Server.Version}}')"; test "$$actual" = "$(DOCKER_VERSION)" || { echo "Docker Engine $(DOCKER_VERSION) required; found $$actual" >&2; exit 1; }
	@actual="$$(kind version | awk '{print $$2}')"; test "$$actual" = "$(KIND_VERSION)" || { echo "Kind $(KIND_VERSION) required; found $$actual" >&2; exit 1; }
	@actual="$$(kubectl version --client --output=yaml | awk '/gitVersion:/ {print $$2; exit}')"; test "$$actual" = "$(KUBECTL_VERSION)" || { echo "kubectl $(KUBECTL_VERSION) required; found $$actual" >&2; exit 1; }
	@printf '%s\n' "$(PLATFORM)" | grep -Eq '^linux/(amd64|arm64)$$' || { echo "PLATFORM must be linux/amd64 or linux/arm64" >&2; exit 1; }

verify-lock:
	@sh "$(CURDIR)/../../docs/development/verify-dev-stack-image-locks.sh" exact "$(CURDIR)/stack.yaml" $(CORE_IMAGES)
	@sh "$(CURDIR)/../../docs/development/verify-dev-stack-image-locks.sh" common "$(CURDIR)/../compose/compose.yaml" "$(CURDIR)/stack.yaml" $(CORE_IMAGES)
	@printf '%s\n' "$(PROBE_IMAGE)" | grep -Eq '^[^[:space:]@]+:[^[:space:]@]+@sha256:[0-9a-f]{64}$$' || { echo "PROBE_IMAGE must be pinned by tag and SHA-256 digest" >&2; exit 1; }
	@printf '%s\n' "$(NODE_IMAGE)" | grep -Eq '^kindest/node:v[0-9]+\.[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$$'
	@printf '%s\n' "$(PROBE_IMAGE)" | grep -Eq '^busybox:[^@]+@sha256:[0-9a-f]{64}$$'
	@test "$$(printf '%s\n' "$(NODE_IMAGE)" | sed -E 's|^kindest/node:(v[0-9]+\.[0-9]+\.[0-9]+)@.*$$|\1|')" = "$(KUBERNETES_VERSION)" || { echo "NODE_IMAGE tag must match KUBERNETES_VERSION" >&2; exit 1; }
	@sh "$(ARCHIVE_HELPER)" verify-kustomize-tags "$(CURDIR)/kustomization.yaml" $(TAG_CORE_IMAGES)

check-images: check verify-lock
	@if [ -f "$(OFFLINE_LOCK)" ]; then \
		sh "$(ARCHIVE_HELPER)" verify-ids "$(OFFLINE_LOCK)" $(NODE_IMAGE) $(IMAGES); \
	else \
		for image in $(NODE_IMAGE) $(IMAGES); do \
			digest="$${image##*@}"; \
			docker image inspect "$$image" >/dev/null 2>&1 || { echo "missing local image: $$image" >&2; echo "run 'make -C deploy/kind images' while connected, or load-images" >&2; exit 1; }; \
			docker image inspect --format '{{join .RepoDigests "\n"}}' "$$image" | grep -Fq "@$$digest" || { echo "RepoDigest mismatch: $$image" >&2; exit 1; }; \
		done; \
	fi

config: check verify-lock
	@kubectl kustomize "$(CURDIR)" >/dev/null

images: check verify-lock
	@rm -f "$(OFFLINE_LOCK)"
	@for image in $(NODE_IMAGE) $(IMAGES); do docker pull --platform "$(PLATFORM)" "$$image" || exit 1; done
	@sh "$(ARCHIVE_HELPER)" tag $(NODE_IMAGE) $(IMAGES)

save-images: check-images
	@sh "$(ARCHIVE_HELPER)" write-ids "$(IDS)" "$(PLATFORM)" $(NODE_IMAGE) $(IMAGES)
	@docker image save --platform "$(PLATFORM)" --output "$(ARCHIVE)" $(TAG_NODE_IMAGE) $(TAG_IMAGES)
	@if command -v sha256sum >/dev/null 2>&1; then sha256sum "$(ARCHIVE)" | awk '{print $$1}' > "$(CHECKSUM)"; else shasum -a 256 "$(ARCHIVE)" | awk '{print $$1}' > "$(CHECKSUM)"; fi
	@if command -v sha256sum >/dev/null 2>&1; then sha256sum "$(IDS)" | awk '{print $$1}' > "$(IDS_CHECKSUM)"; else shasum -a 256 "$(IDS)" | awk '{print $$1}' > "$(IDS_CHECKSUM)"; fi
	@printf '%s\n' "$(PLATFORM)" > "$(PLATFORM_FILE)"

load-images: check
	@test -f "$(ARCHIVE)" || { echo "archive not found: $(ARCHIVE)" >&2; exit 1; }
	@test -f "$(CHECKSUM)" || { echo "checksum not found: $(CHECKSUM)" >&2; exit 1; }
	@test -f "$(PLATFORM_FILE)" || { echo "platform metadata not found: $(PLATFORM_FILE)" >&2; exit 1; }
	@test -f "$(IDS)" || { echo "image ID manifest not found: $(IDS)" >&2; exit 1; }
	@test -f "$(IDS_CHECKSUM)" || { echo "image ID manifest checksum not found: $(IDS_CHECKSUM)" >&2; exit 1; }
	@test "$$(tr -d '[:space:]' < "$(PLATFORM_FILE)")" = "$(PLATFORM)" || { echo "archive platform does not match $(PLATFORM)" >&2; exit 1; }
	@expected="$$(tr -d '[:space:]' < "$(CHECKSUM)")"; if command -v sha256sum >/dev/null 2>&1; then actual="$$(sha256sum "$(ARCHIVE)" | awk '{print $$1}')"; else actual="$$(shasum -a 256 "$(ARCHIVE)" | awk '{print $$1}')"; fi; test "$$actual" = "$$expected" || { echo "archive SHA-256 mismatch" >&2; exit 1; }
	@expected="$$(tr -d '[:space:]' < "$(IDS_CHECKSUM)")"; if command -v sha256sum >/dev/null 2>&1; then actual="$$(sha256sum "$(IDS)" | awk '{print $$1}')"; else actual="$$(shasum -a 256 "$(IDS)" | awk '{print $$1}')"; fi; test "$$actual" = "$$expected" || { echo "image ID manifest SHA-256 mismatch" >&2; exit 1; }
	@docker image load --platform "$(PLATFORM)" --input "$(ARCHIVE)"
	@sh "$(ARCHIVE_HELPER)" verify-ids "$(IDS)" $(NODE_IMAGE) $(IMAGES)
	@cp "$(IDS)" "$(OFFLINE_LOCK)" && chmod 600 "$(OFFLINE_LOCK)"

up: check-images config
	@kind get clusters | grep -Fxq "$(CLUSTER)" || \
		{ node_image="$(NODE_IMAGE)"; docker image inspect "$$node_image" >/dev/null 2>&1 || node_image="$(TAG_NODE_IMAGE)"; kind create cluster --name "$(CLUSTER)" --image "$$node_image" --config "$(CURDIR)/cluster.yaml"; }
	@$(MAKE) verify-cluster
	@archive="$$(mktemp -t threadline-kind-images.XXXXXX.tar)"; trap 'rm -f "$$archive"' EXIT HUP INT TERM; docker image save --platform "$(PLATFORM)" --output "$$archive" $(TAG_IMAGES); for node in $$(kind get nodes --name "$(CLUSTER)"); do docker exec --privileged -i "$$node" ctr --namespace=k8s.io images import --digests --snapshotter=overlayfs - < "$$archive" || exit 1; done
	@kubectl --context "$(CONTEXT)" apply -k "$(CURDIR)"
	@kubectl --context "$(CONTEXT)" --namespace threadline-dev rollout restart deployment/otel-collector deployment/prometheus
	@kubectl --context "$(CONTEXT)" --namespace threadline-dev rollout status deployment/otel-collector --timeout=180s
	@kubectl --context "$(CONTEXT)" --namespace threadline-dev rollout status deployment/prometheus --timeout=180s
	@kubectl --context "$(CONTEXT)" --namespace threadline-dev wait --for=condition=Available deployment --all --timeout=180s
	@$(MAKE) verify-network-policy

verify-cluster: check verify-lock
	@kind get clusters | grep -Fxq "$(CLUSTER)" || { echo "Kind cluster $(CLUSTER) does not exist; run make up" >&2; exit 1; }
	@nodes="$$(kind get nodes --name "$(CLUSTER)")"; test "$$(printf '%s\n' "$$nodes" | sed '/^$$/d' | wc -l | tr -d ' ')" = 1 || { echo "Kind cluster topology does not match the pinned single-node cluster; run make destroy then make up" >&2; exit 1; }; node="$$nodes"; node_image="$(NODE_IMAGE)"; docker image inspect "$$node_image" >/dev/null 2>&1 || node_image="$(TAG_NODE_IMAGE)"; expected="$$(docker image inspect --format '{{.Id}}' "$$node_image")"; actual="$$(docker inspect --format '{{.Image}}' "$$node")"; test -n "$$expected" && test "$$actual" = "$$expected" || { echo "Kind node image does not match NODE_IMAGE; run make destroy then make up" >&2; exit 1; }
	@actual="$$(kubectl --context "$(CONTEXT)" version --output=yaml | awk '/^serverVersion:/ { server=1; next } server && /^  gitVersion:/ { print $$2; exit }')"; test "$$actual" = "$(KUBERNETES_VERSION)" || { echo "Kind Kubernetes $(KUBERNETES_VERSION) required; found $${actual:-unknown}. Run make destroy then make up" >&2; exit 1; }

verify-network-policy: verify-cluster
	@sh "$(CURDIR)/verify-network-policy.sh" "$(CONTEXT)" "$(TAG_PROBE_IMAGE)"

down: check-tools
	@kubectl --context "$(CONTEXT)" delete -k "$(CURDIR)" --ignore-not-found

destroy: check-tools
	@kind delete cluster --name "$(CLUSTER)"

recreate:
	@$(MAKE) destroy
	@$(MAKE) up

status: check-tools
	@kubectl --context "$(CONTEXT)" get pods,services --namespace threadline-dev
	@kubectl --context "$(CONTEXT)" get networkpolicy --all-namespaces
