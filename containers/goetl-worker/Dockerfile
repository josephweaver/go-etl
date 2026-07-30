FROM debian:bookworm AS dmtcp-build

ARG DMTCP_VERSION=4.2.0
ARG DMTCP_COMMIT=f8009ce7b4ad211311ca2f72a929b975e4aa1155
ARG DMTCP_SHA256=3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        g++ \
        make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src/dmtcp

RUN curl -fsSL \
        "https://codeload.github.com/dmtcp/dmtcp/tar.gz/${DMTCP_COMMIT}" \
        -o /tmp/dmtcp.tar.gz \
    && echo "${DMTCP_SHA256}  /tmp/dmtcp.tar.gz" | sha256sum --check --strict \
    && tar -xzf /tmp/dmtcp.tar.gz --strip-components=1 \
    && rm /tmp/dmtcp.tar.gz \
    && ./configure --prefix=/opt/dmtcp \
    && make -j"$(nproc)" \
    && make install \
    && test "$(./bin/dmtcp_launch --version | sed -n '1p')" \
        = "dmtcp_launch (DMTCP) ${DMTCP_VERSION}"

FROM debian:bookworm AS criu-build

ARG CRIU_VERSION=4.2.1
ARG CRIU_COMMIT=f3e4ef5389601ed0893820d5eef1a769a5eee901
ARG CRIU_SHA256=fbe32da7dec8d8443f162b81ff28dae1e75195fd78ca502d94c478504798e5fe

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        libaio-dev \
        libbsd-dev \
        libcap-dev \
        libelf-dev \
        libgnutls28-dev \
        libnet-dev \
        libnl-3-dev \
        libnl-route-3-dev \
        libprotobuf-c-dev \
        libprotobuf-dev \
        libselinux1-dev \
        pkg-config \
        protobuf-c-compiler \
        protobuf-compiler \
        python3-protobuf \
        uuid-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src/criu

RUN curl -fsSL \
        "https://codeload.github.com/checkpoint-restore/criu/tar.gz/${CRIU_COMMIT}" \
        -o /tmp/criu.tar.gz \
    && echo "${CRIU_SHA256}  /tmp/criu.tar.gz" | sha256sum --check --strict \
    && tar -xzf /tmp/criu.tar.gz --strip-components=1 \
    && rm /tmp/criu.tar.gz \
    && make -j"$(nproc)" criu \
    && test "$(./criu/criu --version | sed -n '1p')" = "Version: ${CRIU_VERSION}"

FROM golang:1.26.2-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-linkmode=external" \
    -o /out/goetl-worker ./cmd/worker

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        libaio1 \
        libatomic1 \
        libbsd0 \
        libcap2 \
        libgnutls30 \
        libnet1 \
        libnl-3-200 \
        libnl-route-3-200 \
        libprotobuf-c1 \
        libprotobuf32 \
        libselinux1 \
        libstdc++6 \
        python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /goetl

COPY --from=dmtcp-build /opt/dmtcp/bin /opt/dmtcp/bin
COPY --from=dmtcp-build /opt/dmtcp/lib /opt/dmtcp/lib
COPY --from=criu-build /src/criu/criu/criu /usr/local/sbin/criu
COPY --from=build /out/goetl-worker /goetl/goetl-worker

ENV PATH="/opt/dmtcp/bin:${PATH}"

ENTRYPOINT ["/goetl/goetl-worker"]
