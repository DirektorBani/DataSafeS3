SHELL := /bin/sh

.PHONY: dev down logs test lint dev-local dev-docker-nobuild

dev:
	docker compose up --build

dev-docker-nobuild:
	@echo "On Windows run: scripts\\dev-docker-nobuild.cmd"

dev-local:
	@echo "On Windows run: scripts\\dev-local.cmd"

down:
	docker compose down --remove-orphans

logs:
	docker compose logs -f --tail=200

test:
	go test ./...

lint:
	@echo "==> gofmt"
	@test -z "$$(gofmt -l .)" || (echo "gofmt found issues:" && gofmt -l . && exit 1)
	@echo "==> govet"
	go vet ./...
