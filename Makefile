.PHONY: bump
bump:
	@echo "🚀 Bumping Version"
	git tag $(shell svu patch)
	git push --tags

.PHONY: build
build:
	@echo "🚀 Building Version $(shell svu current)"
	@echo "Generating static library (Swift + CGO)..."
	@go generate ./...
	@echo "Building Go binary with CGO..."
	cd cmd/found && CGO_ENABLED=1 go build -o ../../found .

.PHONY: release
release:
	@echo "🚀 Releasing Version $(shell svu current)"
	goreleaser build --id default --clean --snapshot --single-target --output dist/found

.PHONY: snapshot
snapshot:
	@echo "🚀 Snapshot Version $(shell svu current)"
	goreleaser --clean --timeout 60m --snapshot

.PHONY: vhs
vhs: release
	@echo "📼 VHS Recording"
	@echo "Please ensure you have the 'vhs' command installed."
	vhs < vhs.tape

clean:
	@echo "🧹 Cleaning up..."
	rm -f libFMShim.a
	rm -f libFMShim.o
	rm -f found
	rm -rf dist