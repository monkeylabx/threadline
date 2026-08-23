SQLC ?= sqlc
SQLC_VERSION := 1.31.1
GENERATED_DIR := $(CURDIR)/../services/core/internal/dbgen

.PHONY: check check-sqlc generate generate-check migration-static migration-test

check-sqlc:
	@actual="$$($(SQLC) version)"; test "$$actual" = "v$(SQLC_VERSION)" || { echo "sqlc v$(SQLC_VERSION) required; found $$actual" >&2; exit 1; }

migration-static:
	@sh "$(CURDIR)/tests/migration_roundtrip.sh" --static

migration-test:
	@sh "$(CURDIR)/tests/migration_roundtrip.sh"

generate: check-sqlc migration-static
	@cd "$(CURDIR)" && "$(SQLC)" generate

generate-check: generate
	@snapshot="$$(mktemp -d "$${TMPDIR:-/tmp}/threadline-sqlc.XXXXXX")"; \
	trap 'rm -rf "$$snapshot"' EXIT HUP INT TERM; \
	cp -R "$(GENERATED_DIR)/." "$$snapshot/"; \
	cd "$(CURDIR)"; \
	"$(SQLC)" generate; \
	diff -ru "$$snapshot" "$(GENERATED_DIR)"
	@git -C "$(CURDIR)/.." diff --exit-code -- services/core/internal/dbgen

check: migration-static generate-check
