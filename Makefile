.PHONY: dev build clean run test vet

GO := /usr/local/go/bin/go

dev:
	$(GO) run ./cmd/swipefi

build:
	$(GO) build -o swipefi ./cmd/swipefi

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
