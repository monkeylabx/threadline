.PHONY: build doctor lint proto proto-breaking proto-generate test toolchain-verify verify

build:
	node scripts/workspace.mjs build

doctor:
	node scripts/workspace.mjs doctor

lint:
	node scripts/workspace.mjs lint

proto:
	node scripts/proto.mjs check

proto-breaking:
	node scripts/proto.mjs breaking

proto-generate:
	node scripts/proto.mjs generate

test:
	node scripts/workspace.mjs test

toolchain-verify:
	node scripts/toolchain.mjs verify

verify:
	node scripts/workspace.mjs verify
