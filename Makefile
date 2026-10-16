SHELL := /bin/sh

.PHONY: dev down logs test lint dev-local dev-docker-nobuild openapi openapi-check

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

openapi:
	cp api/openapi/v1.yaml internal/openapi/openapi.yaml
	cp api/openapi/v1.yaml docs/api/openapi.yaml

openapi-check:
	cmp -s api/openapi/v1.yaml internal/openapi/openapi.yaml
	cmp -s api/openapi/v1.yaml docs/api/openapi.yaml
