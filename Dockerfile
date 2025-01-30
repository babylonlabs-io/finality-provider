FROM golang:1.23-alpine AS builder

# Version to build. Default is the Git HEAD.
ARG VERSION="HEAD"

# Use muslc for static libs
ARG BUILD_TAGS="muslc"

# hadolint ignore=DL3018
RUN apk add --no-cache --update openssh git make build-base linux-headers libc-dev \
                                pkgconfig zeromq-dev musl-dev alpine-sdk libsodium-dev \
                                libzmq-static libsodium-static gcc && rm -rf /var/cache/apk/*


# Build
WORKDIR /go/src/github.com/babylonlabs-io/finality-provider
# Cache dependencies
COPY go.mod go.sum /go/src/github.com/babylonlabs-io/finality-provider/
RUN go mod download
# Copy the rest of the files
COPY ./ /go/src/github.com/babylonlabs-io/finality-provider/

# Cosmwasm - Download correct libwasmvm version
SHELL ["/bin/ash", "-eo", "pipefail", "-c"]
RUN WASMVM_VERSION=$(grep github.com/CosmWasm/wasmvm go.mod | cut -d' ' -f2) && \
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc."$(uname -m)".a \
        -O /lib/libwasmvm_muslc."$(uname -m)".a && \
    # verify checksum
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc."$(uname -m)."a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc."$(uname -m)" | cut -d ' ' -f 1)

RUN CGO_LDFLAGS="$CGO_LDFLAGS -lstdc++ -lm -lsodium" \
    CGO_ENABLED=1 \
    BUILD_TAGS=$BUILD_TAGS \
    LINK_STATICALLY=true \
    make build

# FINAL IMAGE
FROM alpine:3.20 AS run

RUN addgroup --gid 1138 -S finality-provider && adduser --uid 1138 -S finality-provider -G finality-provider

# hadolint ignore=DL3018
RUN apk add --no-cache bash curl jq && rm -rf /var/cache/apk/*

COPY --from=builder /go/src/github.com/babylonlabs-io/finality-provider/build/fpd /bin/fpd
COPY --from=builder /go/src/github.com/babylonlabs-io/finality-provider/build/eotsd /bin/eotsd

WORKDIR /home/finality-provider
RUN chown -R finality-provider /home/finality-provider
USER finality-provider
