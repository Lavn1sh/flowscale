FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /flowscale ./cmd/engine

FROM alpine:latest

WORKDIR /
COPY --from=builder /flowscale /flowscale

# We need the migrations folder for when we add automated migrations, 
# or just keeping it available. (Not strictly required for the binary to run if no embedded migrations)
COPY --from=builder /app/migrations /migrations

EXPOSE 8080 8081 2112

ENTRYPOINT ["/flowscale"]
