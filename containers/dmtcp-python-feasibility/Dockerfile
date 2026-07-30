FROM python:3.11.15-slim-bookworm@sha256:28255a3ace7eb4c48bc1b57b90af29e1bc82b4fd6c60614a8e3dce61b87ff941

ARG DMTCP_VERSION=4.2.0
ARG DMTCP_COMMIT=f8009ce7b4ad211311ca2f72a929b975e4aa1155
ARG DMTCP_SHA256=3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0
ARG NUMPY_VERSION=2.4.6
ARG NUMPY_WHEEL=numpy-2.4.6-cp311-cp311-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl
ARG NUMPY_URL=https://files.pythonhosted.org/packages/02/03/74fe2a4cb3817d94d86402f2506554130a2f01414e299b5a843e5a8a957f/numpy-2.4.6-cp311-cp311-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl
ARG NUMPY_SHA256=89cd468399cfd2504718f0ba50e410dca55a170b61a02ad92bb18c8a65186e93

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        g++ \
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

RUN curl -fsSL "${NUMPY_URL}" -o "/tmp/${NUMPY_WHEEL}" \
    && echo "${NUMPY_SHA256}  /tmp/${NUMPY_WHEEL}" \
        | sha256sum --check --strict \
    && python -m pip install --no-cache-dir --no-deps "/tmp/${NUMPY_WHEEL}" \
    && rm "/tmp/${NUMPY_WHEEL}" \
    && test "$(python -c 'import numpy; print(numpy.__version__)')" \
        = "${NUMPY_VERSION}"

ENV MKL_NUM_THREADS=1
ENV NUMEXPR_NUM_THREADS=1
ENV OMP_NUM_THREADS=1
ENV OPENBLAS_NUM_THREADS=1
ENV PATH="/opt/dmtcp/bin:${PATH}"

COPY containers/dmtcp-python-feasibility/testdata/python-checkpoint/fixture.py \
    /opt/goetl-os004/fixture.py

RUN chmod 0555 /opt/goetl-os004/fixture.py \
    && test "$(dmtcp_launch --version | sed -n '1p')" \
        = "dmtcp_launch (DMTCP) ${DMTCP_VERSION}" \
    && test "$(python --version)" = "Python 3.11.15" \
    && python -c 'import numpy; numpy.show_config()' \
    && g++ --version | sed -n '1p' \
    && ldd --version | sed -n '1p'

WORKDIR /checkpoint-test

CMD ["/bin/bash"]
