.PHONY: build test lint clean run install tag

BINARY := bobr
OUTDIR := bin

build:
	@mkdir -p $(OUTDIR)
	go build -ldflags "-X github.com/azuradara/bobr/internal/cli.Version=$(v)" -o $(OUTDIR)/$(BINARY) ./cmd/bobr

test:
	go test -v -race ./...

clean:
	rm -rf $(OUTDIR)

run: build
	./$(OUTDIR)/$(BINARY)

install:
	go install ./cmd/bobr

tag:
	@if [ -z "$(v)" ]; then echo "error: v argument is required (e.g., make tag v=v1.0.0)"; exit 1; fi
	@if [ -n "$$(git status --porcelain)" ]; then echo "error: git state is not clean"; exit 1; fi
	git tag $(v)
	git push origin $(v)

lint:
	golangci-lint run --fix
	golangci-lint fmt
