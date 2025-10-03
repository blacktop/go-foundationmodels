.PHONY: bump
bump:
	@echo "🚀 Bumping Version"
	git tag $(shell svu patch)
	git push --tags

libFMShim.dylib: FoundationModelsShim.swift
	@echo "🚀 Building libFMShim.dylib"
	@echo "Using SDK: $(shell xcrun --show-sdk-path)"
	@echo "Target: arm64-apple-macos26"
	@echo "Output: libFMShim.dylib"
	swiftc \
	-sdk $(shell xcrun --show-sdk-path) \
	-target arm64-apple-macos26 \
	-emit-library \
	-o libFMShim.dylib \
	FoundationModelsShim.swift

.PHONY: build
build:
	@echo "🚀 Building Version $(shell svu current)"
	@echo "Generating static library (Swift + CGO)..."
	@go generate ./...
	@echo "Building Go binary with CGO..."
	cd cmd/found && CGO_ENABLED=1 go build -o ../../found .

.PHONY: build-static
build-static:
	@echo "🚀 Building static version with CGO"
	@echo "Generating static library..."
	go generate ./...
	@echo "Building with CGO enabled..."
	cd cmd/found && CGO_ENABLED=1 go build -o ../../found-static .

.PHONY: release
release: libFMShim.dylib
	@echo "🚀 Releasing Version $(shell svu current)"
	goreleaser build --id default --clean --snapshot --single-target --output dist/found

.PHONY: snapshot
snapshot: libFMShim.dylib
	@echo "🚀 Snapshot Version $(shell svu current)"
	goreleaser --clean --timeout 60m --snapshot

.PHONY: vhs
vhs: release
	@echo "📼 VHS Recording"
	@echo "Please ensure you have the 'vhs' command installed."
	vhs < vhs.tape

clean:
	@echo "🧹 Cleaning up..."
	rm -f libFMShim.dylib
	rm -f libFMShim.a
	rm -f libFMShim.o
	rm -f found
	rm -f found-static
	rm -rf dist