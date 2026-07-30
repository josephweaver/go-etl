FROM rocker/tidyverse@sha256:d6684038a67fc65864c958151d76162a8f005e87f3bc861153e3d3c905f7dbdb

ARG DMTCP_VERSION=4.2.0
ARG DMTCP_COMMIT=f8009ce7b4ad211311ca2f72a929b975e4aa1155
ARG DMTCP_SHA256=3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0
ARG R_PACKAGE_SNAPSHOT=2026-05-20
ARG TIDYVERSE_VERSION=2.0.0
ARG BRMS_VERSION=2.23.0
ARG RSTAN_VERSION=2.32.7
ARG CMDSTANR_VERSION=0.9.0
ARG CMDSTANR_SHA256=55e1d9f7d2dba08771850243944606020c9ca4e682ff53ab6a4e9fbc3032a003
ARG CMDSTAN_VERSION=2.39.0
ARG CMDSTAN_SHA256=ffe03c29c9f139d77deeb156a2a0911ebf0742ae38311c739f4fcea3bc4f8909

USER root

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        g++ \
        jq \
        make \
        procps \
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
        = "dmtcp_launch (DMTCP) ${DMTCP_VERSION}" \
    && rm -rf /src/dmtcp

ENV R_PACKAGE_REPOSITORY="https://packagemanager.posit.co/cran/__linux__/jammy/${R_PACKAGE_SNAPSHOT}"

RUN Rscript -e 'options(repos = c(CRAN = Sys.getenv("R_PACKAGE_REPOSITORY"))); install.packages(c("rstan", "brms"), Ncpus = max(1L, parallel::detectCores(logical = FALSE)))' \
    && test "$(Rscript -e 'cat(as.character(packageVersion("tidyverse")))')" = "${TIDYVERSE_VERSION}" \
    && test "$(Rscript -e 'cat(as.character(packageVersion("brms")))')" = "${BRMS_VERSION}" \
    && test "$(Rscript -e 'cat(as.character(packageVersion("rstan")))')" = "${RSTAN_VERSION}"

RUN curl -fsSL \
        "https://codeload.github.com/stan-dev/cmdstanr/tar.gz/refs/tags/v${CMDSTANR_VERSION}" \
        -o /tmp/cmdstanr.tar.gz \
    && echo "${CMDSTANR_SHA256}  /tmp/cmdstanr.tar.gz" \
        | sha256sum --check --strict \
    && R CMD INSTALL /tmp/cmdstanr.tar.gz \
    && rm /tmp/cmdstanr.tar.gz \
    && test "$(Rscript -e 'cat(as.character(packageVersion("cmdstanr")))')" \
        = "${CMDSTANR_VERSION}"

RUN mkdir -p /opt/cmdstan \
    && curl -fsSL \
        "https://github.com/stan-dev/cmdstan/releases/download/v${CMDSTAN_VERSION}/cmdstan-${CMDSTAN_VERSION}.tar.gz" \
        -o /tmp/cmdstan.tar.gz \
    && echo "${CMDSTAN_SHA256}  /tmp/cmdstan.tar.gz" \
        | sha256sum --check --strict \
    && tar -xzf /tmp/cmdstan.tar.gz -C /opt/cmdstan \
    && rm /tmp/cmdstan.tar.gz \
    && make -C "/opt/cmdstan/cmdstan-${CMDSTAN_VERSION}" -j2 build

ENV CMDSTAN="/opt/cmdstan/cmdstan-${CMDSTAN_VERSION}"
ENV PATH="/opt/dmtcp/bin:${CMDSTAN}/bin:${PATH}"

RUN Rscript -e 'cmdstanr::set_cmdstan_path(Sys.getenv("CMDSTAN")); stopifnot(as.character(cmdstanr::cmdstan_version()) == Sys.getenv("CMDSTAN_VERSION", unset = "2.39.0"))' \
    && dmtcp_launch --version \
    && Rscript --version \
    && g++ --version | sed -n '1p' \
    && ldd --version | sed -n '1p'

COPY containers/dmtcp-r-brms-feasibility/testdata/brms-checkpoint/fixture.R \
    /opt/goetl-os003/fixture.R

RUN chmod 0555 /opt/goetl-os003/fixture.R

WORKDIR /checkpoint-test

CMD ["/bin/bash"]
