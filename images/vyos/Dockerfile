# Supply an installed VyOS qcow2 from the existing VyOS image installer. No
# in-band management NIC, SSH service, or runtime package downloads are added.
FROM ghcr.io/srl-labs/vrnetlab-base:0.3.0
ARG SOURCE_REVISION
ARG HELPER_SHA256
ARG VYOS_SHA256
ARG QGA_MEDIA_SHA256
ARG IMAGE=vyos.qcow2
ARG QGA_MEDIA=qga-tools.iso
ENV QEMU_MEMORY=2048 QEMU_SMP=2
COPY --from=labcontainers-control labcontainers-guest /labcontainers-guest
COPY --from=labcontainers-disk ${IMAGE} /vyos.qcow2
COPY --from=labcontainers-media ${QGA_MEDIA} /qga-tools.iso
COPY --from=labcontainers-vyos launcher.py /launch.py
COPY --from=labcontainers-windows launcher.py /qga_vm.py
COPY --from=labcontainers-common interfaces.py /interfaces.py
RUN test -n "$SOURCE_REVISION" && test -n "$HELPER_SHA256" && test -n "$VYOS_SHA256" && test -n "$QGA_MEDIA_SHA256" && \
    printf '%s  /labcontainers-guest\n%s  /vyos.qcow2\n%s  /qga-tools.iso\n' "$HELPER_SHA256" "$VYOS_SHA256" "$QGA_MEDIA_SHA256" | sha256sum -c - && \
    chmod 0755 /labcontainers-guest /launch.py
LABEL appmana.labcontainers.revision="${SOURCE_REVISION}" \
      appmana.labcontainers.guest-helper-sha256="${HELPER_SHA256}" \
      appmana.vyos.disk-sha256="${VYOS_SHA256}" \
      appmana.vyos.qga-media-sha256="${QGA_MEDIA_SHA256}"
ENTRYPOINT ["uv", "run", "/launch.py"]
