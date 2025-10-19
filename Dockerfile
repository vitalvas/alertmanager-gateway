FROM debian:13

ARG TARGETOS
ARG TARGETARCH

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY bin/alertmanager-gateway-${TARGETOS}-${TARGETARCH} /app/alertmanager-gateway

EXPOSE 8080

ENTRYPOINT ["/app/alertmanager-gateway"]
CMD ["--config", "/etc/alertmanager-gateway/config.yaml"]
