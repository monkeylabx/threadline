.PHONY: build doctor lint test toolchain-verify verify

build:
	node scripts/workspace.mjs build

doctor:
	node scripts/workspace.mjs doctor

lint:
	node scripts/workspace.mjs lint

test:
	node scripts/workspace.mjs test

toolchain-verify:
	node scripts/toolchain.mjs verify

verify:
	node scripts/workspace.mjs verify
