## This Dockerfile builds the Crypto Broker Operator.
## The operator watches CryptoBroker resources and injects the
## crypto-broker-server sidecar into the target Deployments.

ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.24

## Build stage
# --platform=$BUILDPLATFORM builds on the builder's native architecture and
# cross-compiles to the requested TARGETOS/TARGETARCH (both injected by buildx),
# so a single Dockerfile can produce amd64 and arm64 images.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

# -trimpath removes local filesystem paths for reproducible builds; -s -w drop
# the symbol table and DWARF debug info to shrink the binary.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o operator ./

## Deploy stage
## Use distroless/static:nonroot (the server/cli images use scratch). It ships a
## non-root user (uid 65532) that matches the Deployment's security context
## (runAsNonRoot: true, runAsUser: 65532, readOnlyRootFilesystem: true), plus a
## CA-certificate bundle and tzdata. Note: TLS to the Kubernetes API is verified
## with the ServiceAccount CA that Kubernetes mounts at
## /var/run/secrets/kubernetes.io/serviceaccount/ca.crt, so the image CA bundle is
## only relevant if the operator ever calls other, external TLS endpoints.
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=build /app/operator /operator

USER 65532:65532

ENTRYPOINT ["/operator"]
