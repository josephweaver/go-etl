FROM debian:bookworm-slim

ARG GOET_RCLONE_RELEASE=1.71.2
ARG RCLONE_SHA256=ab9fa5877cee91c64fdfd61a27028a458cf618b39259e5c371dc2ec34a12e415

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && curl -fsSL \
        "https://downloads.rclone.org/v${GOET_RCLONE_RELEASE}/rclone-v${GOET_RCLONE_RELEASE}-linux-amd64.zip" \
        -o /tmp/rclone.zip \
    && echo "${RCLONE_SHA256}  /tmp/rclone.zip" | sha256sum --check --strict \
    && unzip -q /tmp/rclone.zip -d /tmp \
    && install -m 0755 \
        "/tmp/rclone-v${GOET_RCLONE_RELEASE}-linux-amd64/rclone" \
        /usr/local/bin/rclone \
    && test "$(rclone version | sed -n '1p')" = "rclone v${GOET_RCLONE_RELEASE}" \
    && rm -rf /tmp/rclone.zip "/tmp/rclone-v${GOET_RCLONE_RELEASE}-linux-amd64" \
        /var/lib/apt/lists/*

ENTRYPOINT ["/usr/local/bin/rclone"]
