FROM alpine
WORKDIR /
COPY renovate-metrics /usr/bin/renovate-metrics
USER 65532:65532

ENTRYPOINT ["/usr/bin/renovate-metrics"]
