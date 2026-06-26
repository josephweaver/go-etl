ARG SLURM_BASE_IMAGE=slurm-docker-cluster:25.11.4
FROM ${SLURM_BASE_IMAGE}

ARG SINGULARITY_CE_VERSION=4.1.2
ARG SINGULARITY_CE_RPM_URL=https://github.com/sylabs/singularity/releases/download/v4.1.2/singularity-ce-4.1.2-1.el9.x86_64.rpm

RUN dnf install -y --allowerasing \
        "${SINGULARITY_CE_RPM_URL}" \
        openssh-clients \
        openssh-server \
    && dnf clean all \
    && rm -rf /var/cache/dnf

RUN singularity --version | grep "singularity-ce version ${SINGULARITY_CE_VERSION}"

RUN useradd --create-home --shell /bin/bash goetl \
    && mkdir -p /home/goetl/.ssh /data/goetl /run/sshd \
    && chmod 700 /home/goetl/.ssh \
    && chown -R goetl:goetl /home/goetl /data/goetl \
    && ssh-keygen -A \
    && printf '%s\n' \
        'PasswordAuthentication no' \
        'PubkeyAuthentication yes' \
        'PermitRootLogin no' \
        'AllowUsers goetl' \
        'AuthorizedKeysFile .ssh/authorized_keys' \
        'UsePAM no' \
        > /etc/ssh/sshd_config.d/goetl-fake-hpcc.conf \
    && /usr/sbin/sshd -t

EXPOSE 22
