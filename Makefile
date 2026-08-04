.PHONY: build doctor lint test verify

build:
	node scripts/workspace.mjs build

doctor:
	node scripts/workspace.mjs doctor

lint:
	node scripts/workspace.mjs lint

test:
	node scripts/workspace.mjs test

verify:
	node scripts/workspace.mjs verify
