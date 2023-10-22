# Go releaser dockerfile
FROM alpine as certs
RUN apk update && apk add ca-certificates
FROM scratch
COPY slack-proxy /usr/bin/slack-proxy
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Some of this borrowed from fortiotel's Dockerfile - would need to use tracing for this to be useful
# TODO: add tracing?
ENV OTEL_SERVICE_NAME "slack-proxy"
# Assumes you added --collector.otlp.enabled=true to your Jaeger deployment
ENV OTEL_EXPORTER_OTLP_ENDPOINT http://jaeger-collector.istio-system.svc.cluster.local:4317
EXPOSE 9090
EXPOSE 8080
# configmap (dynamic flags)
VOLUME /etc/slack-proxy
# data files etc
VOLUME /var/lib/slack-proxy
WORKDIR /var/lib/slack-proxy
ENTRYPOINT ["/usr/bin/slack-proxy"]
# TODO: Need to pass the token as secret or env
CMD ["-config-dir", "/etc/slack-proxy"]
