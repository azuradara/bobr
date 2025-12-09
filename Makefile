.PHONY: build test lint clean run

BINARY := bobr
OUTDIR := bin

build:
	@mkdir -p $(OUTDIR)
	go build -o $(OUTDIR)/$(BINARY) ./cmd/bobr

test:
	go test -v -race ./...

clean:
	rm -rf $(OUTDIR)

run: build
	./$(OUTDIR)/$(BINARY)

lint:
	golangci-lint run --fix
	golangci-lint fmt
