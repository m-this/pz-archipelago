include versions.env
export

# The apworld owns the release version: everything that states a version of
# this project reads it from here.
RELEASE_VERSION := $(shell sed -n 's/.*"world_version": "\([^"]*\)".*/\1/p' apworld/planetzoo/archipelago.json)

DIST := dist
CACHE := .cache
AP_DIR := $(CACHE)/archipelago-$(ARCHIPELAGO_VERSION)
# The apworld's container format, which Archipelago 0.6.7 and later read.
APWORLD_CONTAINER_VERSION := 7

GOFUMPT := go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
GOLANGCI_LINT := $(CACHE)/bin/golangci-lint-$(GOLANGCI_LINT_VERSION)
GO_SRC := $$(find . -type f -name '*.go' -not -path './$(CACHE)/*')

.PHONY: version-check help check fmt fmt-check vet lint test export import apworld-test apworld tracker launcher dist clean

help:
	@echo "pz-archipelago $(RELEASE_VERSION)"
	@echo "  make check         Everything CI runs"
	@echo "  make test          Go tests"
	@echo "  make lint          golangci-lint"
	@echo "  make fmt           gofumpt the Go sources"
	@echo "  make export        Regenerate apworld/planetzoo/data from gamedata/"
	@echo "  make import UPSTREAM=<worlds/planetzoo>"
	@echo "                     Rewrite the gamedata tables from an upstream apworld"
	@echo "  make apworld-test  Run the apworld's tests inside Archipelago $(ARCHIPELAGO_VERSION)"
	@echo "  make dist          pzap.exe, planetzoo.apworld and the PopTracker pack in $(DIST)/"

check: fmt-check lint test apworld-test dist

fmt:
	$(GOFUMPT) -w $(GO_SRC)

fmt-check:
	@files="$$($(GOFUMPT) -l $(GO_SRC))"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

# The exe is Windows-only code in places (Oodle, the registry, the process
# list), so the lint runs once per target system.
$(GOLANGCI_LINT):
	GOBIN=$(CURDIR)/$(CACHE)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv $(CACHE)/bin/golangci-lint $@

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...
	GOOS=windows $(GOLANGCI_LINT) run ./...

test:
	go test -race -shuffle=on ./...

export:
	go run ./gamedata/cmd/export apworld/planetzoo/data

import:
	@test -n "$(UPSTREAM)" || { echo "usage: make import UPSTREAM=<ArchipelagoPZ>/worlds/planetzoo"; exit 2; }
	go run ./gamedata/cmd/importapworld $(UPSTREAM) gamedata
	$(MAKE) export

# --- The apworld ---
#
# Its tests need Archipelago around them. The release tarball is unpacked once
# into $(CACHE), the world is copied into its worlds/, and uv supplies the
# pinned Python with Archipelago's requirements minus the GUI and the native
# speedups, which nothing headless needs.

$(AP_DIR):
	mkdir -p $(CACHE)
	curl -fsSL "https://github.com/ArchipelagoMW/Archipelago/archive/refs/tags/$(ARCHIPELAGO_VERSION).tar.gz" \
		| tar xz -C $(CACHE)
	mv $(CACHE)/Archipelago-$(ARCHIPELAGO_VERSION) $@
	grep -vE '^(kivy|kivymd|Pymem|cython|cymem|pyshortcuts)|^#|^$$' $@/requirements.txt > $@/requirements-headless.txt

apworld-test: $(AP_DIR)
	rm -rf $(AP_DIR)/worlds/planetzoo
	cp -r apworld/planetzoo $(AP_DIR)/worlds/planetzoo
	cd $(AP_DIR) && SKIP_REQUIREMENTS_UPDATE=1 uv run --no-project --python $(PYTHON_VERSION) \
		--with-requirements requirements-headless.txt \
		python -m unittest discover -s worlds/planetzoo/test -t .

# --- Artifacts ---

# A release tag must name the version the tree states.
version-check:
	@test "$(TAG)" = "$(RELEASE_VERSION)" || { echo "tag $(TAG) is not $(RELEASE_VERSION)"; exit 1; }

apworld:
	go run ./launcher/cmd/packagezip apworld apworld/planetzoo $(DIST)/planetzoo.apworld \
		-container-version $(APWORLD_CONTAINER_VERSION)

tracker:
	go run ./launcher/cmd/packagezip tree tracker $(DIST)/planetzoo-poptracker.zip

launcher:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X main.version=$(RELEASE_VERSION)" \
		-o $(DIST)/pzap.exe ./launcher/cmd/pzap

dist: apworld tracker launcher

clean:
	rm -rf $(DIST)
