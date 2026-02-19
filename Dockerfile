FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 go build -ldflags="-s -w -X main.version=docker" -o /servprobe ./cmd/servprobe

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata iputils

COPY --from=builder /servprobe /usr/local/bin/servprobe

RUN mkdir -p /data
VOLUME /data

EXPOSE 8893

ENTRYPOINT ["servprobe"]
CMD ["serve", "--config", "/etc/servprobe/config.yml", "--db", "/data/servprobe.db"]
