ARG VRNETLAB_IMAGE=vrnetlab/canonical_ubuntu:jammy
FROM ${VRNETLAB_IMAGE}
COPY labcontainers-guest /labcontainers-guest
COPY launcher.py /labcontainers-launch.py
ENTRYPOINT ["uv", "run", "/labcontainers-launch.py"]
