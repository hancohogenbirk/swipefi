.PHONY: dev build clean run test vet install-hooks

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

# Git hooks. Run once after cloning. Hooks live in scripts/git-hooks/ so they
# stay version-controlled; this target symlinks them into .git/hooks/.
install-hooks:
	@for hook in pre-commit pre-push; do \
		ln -sf ../../scripts/git-hooks/$$hook .git/hooks/$$hook ; \
		echo "installed .git/hooks/$$hook -> ../../scripts/git-hooks/$$hook" ; \
	done
