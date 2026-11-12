FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN sed -i 's/\t\treturn 0, nil/\t\treturn tw.ResponseWriter.Write(b)/' internal/adapters/httpserver/middleware.go && \
    sed -i 's/\tc := goredis.NewClient(&goredis.Options{/\tc := goredis.NewClient(\&goredis.Options{\n\t\tProtocol: 2,/' internal/infrastructure/storage/redis/*.go && \
    sed -i 's/strings.Contains(msg, "Unknown Index name")/strings.Contains(strings.ToLower(msg), "unknown index name")/' internal/infrastructure/storage/redis/*.go && \
    grep -q 'return tw.ResponseWriter.Write(b)' internal/adapters/httpserver/middleware.go && \
    grep -q '"unknown index name"' internal/infrastructure/storage/redis/*.go && \
    grep -q 'Protocol: 2' internal/infrastructure/storage/redis/*.go && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/gateway /app/gateway
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
