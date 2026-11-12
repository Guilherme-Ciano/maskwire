FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/maskwire ./cmd/maskwire && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/mockllm ./cmd/mockllm && \
    mkdir -p /out/audit

FROM gcr.io/distroless/static-debian13:nonroot AS mockllm
COPY --from=build /out/mockllm /usr/local/bin/mockllm
USER 65532:65532
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/mockllm"]

FROM gcr.io/distroless/static-debian13:nonroot AS maskwire
COPY --from=build /out/maskwire /usr/local/bin/maskwire
COPY --from=build --chown=65532:65532 /out/audit /var/lib/maskwire/audit
USER 65532:65532
EXPOSE 8090 9090
ENTRYPOINT ["/usr/local/bin/maskwire"]
CMD ["serve"]
