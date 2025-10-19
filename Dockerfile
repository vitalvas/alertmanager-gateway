FROM debian:13

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/vitalvas/alertmanager-gateway"
LABEL org.opencontainers.image.description="Universal adapter for Prometheus Alertmanager webhooks that transforms and routes alerts to various third-party notification systems"
LABEL org.opencontainers.image.licenses="MIT"

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY bin/alertmanager-gateway-${TARGETOS}-${TARGETARCH} /app/alertmanager-gateway

EXPOSE 8080

ENTRYPOINT ["/app/alertmanager-gateway"]
CMD ["--config", "/etc/alertmanager-gateway/config.yaml"]
