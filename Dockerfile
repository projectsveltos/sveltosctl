# Build the manager binary
FROM golang:1.27.1 AS builder

ARG ARCH
ARG LDFLAGS
ARG BUILDOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY api/ api
COPY cmd/ cmd
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=$BUILDOS GOARCH=$TARGETARCH GO111MODULE=on go build -ldflags "$LDFLAGS" -a -o sveltosctl cmd/sveltosctl/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

ARG GIT_VERSION=unknown

LABEL org.opencontainers.image.source="https://github.com/projectsveltos/sveltosctl" \
      org.opencontainers.image.url="https://projectsveltos.io" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.vendor="projectsveltos" \
      org.opencontainers.image.title="sveltosctl" \
      org.opencontainers.image.description="Command line tool to visualize information on deployed Sveltos features." \
      org.opencontainers.image.version="$GIT_VERSION" \
      org.opencontainers.image.revision="$GIT_VERSION"

WORKDIR /
COPY --from=builder /workspace/sveltosctl .
USER nonroot:nonroot

ENTRYPOINT ["/sveltosctl"]
