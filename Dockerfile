FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates \
    && go env -w GOPROXY=https://goproxy.cn,direct

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME} -s -w" \
    -o /bin/shortlink ./cmd/shortlink/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -g '' shortlink

COPY --from=builder /bin/shortlink /usr/local/bin/shortlink

RUN mkdir -p /data && chown shortlink /data

USER shortlink

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

VOLUME ["/data"]

ENTRYPOINT ["shortlink"]
CMD ["-config", "/etc/shortlink/config.yaml"]