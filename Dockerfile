FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN for dir in ./cmd/*/; do go build -o /out/$(basename $dir) $dir; done

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /out/ .
