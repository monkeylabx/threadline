SQLC ?= sqlc
GO ?= go
SQLC_VERSION := 1.31.1
CORE_GENERATED_DIR := $(CURDIR)/../services/core/internal/dbgen
WORKER_GENERATED_DIR := $(CURDIR)/../services/worker/internal/dbgen

.PHONY: check check-sqlc generate generate-check migration-static migration-test organization-test organization-go-test member-test member-go-test space-test space-go-test channel-dm-test channel-dm-go-test channel-membership-test channel-membership-go-test resource-acl-test resource-acl-go-test authorization-current-test authorization-current-go-test channel-archive-go-test transactional-outbox-test transactional-outbox-go-test transactional-outbox-worker-ops-test transactional-outbox-worker-go-test

check-sqlc:
	@actual="$$($(SQLC) version)"; test "$$actual" = "v$(SQLC_VERSION)" || { echo "sqlc v$(SQLC_VERSION) required; found $$actual" >&2; exit 1; }

migration-static:
	@sh "$(CURDIR)/tests/migration_roundtrip.sh" --static

migration-test:
	@sh "$(CURDIR)/tests/migration_roundtrip.sh"

transactional-outbox-test:
	@sh "$(CURDIR)/tests/transactional_outbox_persistence.sh"

transactional-outbox-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/outboxstore -run '^TestInsert' -count=1

transactional-outbox-worker-ops-test:
	@sh "$(CURDIR)/tests/transactional_outbox_worker_ops.sh"

transactional-outbox-worker-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly -race \
		./worker/internal/outboxdb -run '^TestAdapterPostgres' -count=1

organization-test:
	@sh "$(CURDIR)/tests/organization_persistence.sh"

organization-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/dbgen -run '^TestOrganizationPersistenceIntegration$$' -count=1

member-test:
	@sh "$(CURDIR)/tests/member_persistence.sh"

member-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/dbgen -run '^TestMemberPersistenceIntegration$$' -count=1

space-test:
	@sh "$(CURDIR)/tests/space_persistence.sh"

space-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/dbgen -run '^TestSpacePersistenceIntegration$$' -count=1

channel-dm-test:
	@sh "$(CURDIR)/tests/channel_dm_persistence.sh"

channel-dm-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/dbgen -run '^TestChannelDirectMessagePersistenceIntegration$$' -count=1

channel-membership-test:
	@sh "$(CURDIR)/tests/channel_membership_persistence.sh"

channel-membership-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/dbgen -run '^TestChannelMembershipPersistenceIntegration$$' -count=1

resource-acl-test:
	@sh "$(CURDIR)/tests/resource_acl_persistence.sh"

resource-acl-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/authorization/aclstore -count=1

authorization-current-test:
	@sh "$(CURDIR)/tests/authorization_current.sh"

authorization-current-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly \
		./core/internal/authorization/current -count=1

channel-archive-go-test:
	@test -n "$$THREADLINE_TEST_POSTGRES_DSN" || { echo "THREADLINE_TEST_POSTGRES_DSN is required" >&2; exit 1; }
	@cd "$(CURDIR)/../services" && GOTOOLCHAIN=local "$(GO)" test -mod=readonly -race \
		./core/internal/channelcommand -count=1

generate: check-sqlc migration-static
	@cd "$(CURDIR)" && "$(SQLC)" generate

generate-check: generate
	@snapshot="$$(mktemp -d "$${TMPDIR:-/tmp}/threadline-sqlc.XXXXXX")"; \
	trap 'rm -rf "$$snapshot"' EXIT HUP INT TERM; \
	mkdir "$$snapshot/core" "$$snapshot/worker"; \
	cp -R "$(CORE_GENERATED_DIR)/." "$$snapshot/core/"; \
	cp -R "$(WORKER_GENERATED_DIR)/." "$$snapshot/worker/"; \
	cd "$(CURDIR)"; \
	"$(SQLC)" generate; \
	diff -ru "$$snapshot/core" "$(CORE_GENERATED_DIR)"; \
	diff -ru "$$snapshot/worker" "$(WORKER_GENERATED_DIR)"
	@git -C "$(CURDIR)/.." diff --exit-code -- \
		services/core/internal/dbgen services/worker/internal/dbgen

check: migration-static generate-check
