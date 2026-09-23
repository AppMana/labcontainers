ARG VRNETLAB_IMAGE=vrnetlab/canonical_ubuntu:jammy-qga
FROM ${VRNETLAB_IMAGE}
COPY labcontainers-guest /labcontainers-guest
COPY launcher.py /labcontainers-launch.py
COPY --from=labcontainers-common interfaces.py /interfaces.py
COPY offline-network.yaml /extra-network.yaml
ENTRYPOINT ["uv", "run", "/labcontainers-launch.py"]
