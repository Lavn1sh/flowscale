.PHONY: build test up down psql migrate-up migrate-down

build:
	go build -o bin/flowscale ./cmd/...

test:
	go test -v -race ./...

up:
	docker-compose up -d

down:
	docker-compose down

psql:
	docker exec -it flowscale-postgres-1 psql -U flowscale -d flowscale

migrate-up:
	docker run --rm -v "$(CURDIR)/migrations:/migrations" migrate/migrate -path=/migrations/ -database postgres://flowscale:password@host.docker.internal:5432/flowscale?sslmode=disable up

migrate-down:
	docker run --rm -v "$(CURDIR)/migrations:/migrations" migrate/migrate -path=/migrations/ -database postgres://flowscale:password@host.docker.internal:5432/flowscale?sslmode=disable down -all
