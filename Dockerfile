# Build the manager binary
FROM golang:1.24.1 AS builder

ARG ARCH
ARG GIT_VERSION=unknown
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

LABEL name="Sveltos CLI tool" \
      vendor="Projectsveltos" \
      version=$GIT_VERSION \
      release="1" \
      summary="Sveltos CLI tool" \
      description="sveltoctl is a command line tool used to visualize information on deployed features." \
      maintainer=""

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/sveltosctl .
USER nonroot:nonroot

ENTRYPOINT ["/sveltosctl"]
