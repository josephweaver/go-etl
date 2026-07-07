FROM golang:1.26.2-bookworm AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gdal-bin \
        libgdal-dev \
        python3 \
        g++ \
        make \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -tags gdal -trimpath -o /out/goetl-worker ./cmd/worker

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gdal-bin \
        python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /goetl

COPY --from=build /out/goetl-worker /goetl/goetl-worker

ENTRYPOINT ["/goetl/goetl-worker"]
