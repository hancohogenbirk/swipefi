.PHONY: dev build clean run test vet

GO := /usr/local/go/bin/go
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X swipefi/internal/version.Commit=$(GIT_SHA) -X swipefi/internal/version.BuildDate=$(BUILD_DATE)

dev:
	$(GO) run -ldflags="$(LDFLAGS)" ./cmd/swipefi

build:
	$(GO) build -ldflags="$(LDFLAGS)" -o swipefi ./cmd/swipefi

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -f swipefi

tidy:
	$(GO) mod tidy

# Frontend
.PHONY: frontend frontend-dev frontend-install

frontend-install:
	cd web && npm install

frontend:
	cd web && npm run build

frontend-dev:
	cd web && npm run dev
