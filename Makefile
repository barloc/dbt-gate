include makefile.env

IMAGE ?= barloc/dbt-gate
VERSION ?= latest
CONFIG ?= config.example.yaml

.PHONY: test build run smoke up down docker-build docker-push

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o build/local/$(SERVICE_NAME) .

run: build
	./build/local/$(SERVICE_NAME) --config $(CONFIG)

smoke:
	@curl -sfS http://127.0.0.1:8090/health >/dev/null
	@curl -sfS -X POST http://127.0.0.1:8080/v1/search \
		-H 'Content-Type: application/json' \
		-d '{"query":"users"}' | grep -q '"ok":true'
	@echo "smoke ok"

up:
	docker compose up --build -d

down:
	docker compose down

docker-build:
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-push: docker-build
	docker push $(IMAGE):$(VERSION)
	@if [ "$(VERSION)" != "latest" ]; then docker push $(IMAGE):latest; fi
