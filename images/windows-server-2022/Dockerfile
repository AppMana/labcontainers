FROM ghcr.io/srl-labs/vrnetlab-base:0.3.0 AS runtime

COPY labcontainers-guest /labcontainers-guest
COPY launcher.py /launch.py
COPY --from=labcontainers-common interfaces.py /interfaces.py
RUN chmod 0755 /labcontainers-guest /launch.py

EXPOSE 5000 10000-10099
ENTRYPOINT ["uv", "run", "/launch.py"]

# Publish --target runtime without licensed guest media or Packer credentials.
# Consumers bind their own prepared qcow2 at the container root using the
# native Containerlab Binds API. The default target still embeds a local disk.
FROM runtime AS image
ARG IMAGE=windows-server-2022-labcontainers.qcow2
COPY ${IMAGE} /windows-server-2022-labcontainers.qcow2
