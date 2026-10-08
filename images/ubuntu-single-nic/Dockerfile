ARG VRNETLAB_IMAGE=vrnetlab/canonical_ubuntu:jammy-qga
FROM ${VRNETLAB_IMAGE}
COPY labcontainers-guest /labcontainers-guest
COPY launcher.py /labcontainers-launch.py
COPY --from=labcontainers-common interfaces.py /interfaces.py
COPY offline-network.yaml /extra-network.yaml
# The base image's venv is already synced; `uv run` would only add a resident
# parent process (34 MiB per VM) and re-resolve on every health probe.
HEALTHCHECK CMD ["/.venv/bin/python3", "/healthcheck.py"]
ENTRYPOINT ["/.venv/bin/python3", "/labcontainers-launch.py"]
