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
build: libFMShim.dylib
	@echo "🚀 Building Version $(shell svu current)"
	go build -o found ./cmd/found

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
	rm -f found
	rm -rf dist