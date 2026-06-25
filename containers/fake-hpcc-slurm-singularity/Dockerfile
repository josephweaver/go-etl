ARG SLURM_BASE_IMAGE=slurm-docker-cluster:25.11.4
FROM ${SLURM_BASE_IMAGE}

ARG SINGULARITY_CE_VERSION=4.1.2
ARG SINGULARITY_CE_RPM_URL=https://github.com/sylabs/singularity/releases/download/v4.1.2/singularity-ce-4.1.2-1.el9.x86_64.rpm

RUN dnf install -y --allowerasing "${SINGULARITY_CE_RPM_URL}" \
    && dnf clean all \
    && rm -rf /var/cache/dnf

RUN singularity --version | grep "singularity-ce version ${SINGULARITY_CE_VERSION}"
