FROM node:24.21.0-alpine AS ui-builder
RUN apk add --no-cache bash
WORKDIR /src
COPY . .
RUN corepack enable && corepack prepare yarn@4.10.3 --activate
RUN ./scripts/frontend.sh

FROM golang:1.24.10-alpine AS go-builder
RUN apk add --no-cache bash git
WORKDIR /src
COPY . .
RUN rm -rf application/webserver/frontend/files && mkdir -p application/webserver/frontend/files
COPY --from=ui-builder /src/application/webserver/frontend/files/ ./application/webserver/frontend/files/
RUN ./scripts/frontend.sh validate
RUN for attempt in 1 2 3; do \
      go mod download && break; \
      if [ "$attempt" -eq 3 ]; then exit 1; fi; \
      echo "go mod download failed; retrying in 5 seconds ($attempt/3)" >&2; \
      sleep 5; \
    done
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /gatesentry-bin .

FROM alpine:3.22
ARG VCS_REF=unknown
LABEL org.opencontainers.image.revision=$VCS_REF
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /usr/local/gatesentry
COPY --from=go-builder /gatesentry-bin ./
RUN mkdir -p /usr/local/gatesentry/gatesentry
EXPOSE 53/udp 53/tcp 10413 10786
# The unauthenticated readiness endpoint on the fixed web admin port; busybox
# wget is present in the alpine base image.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:10786/health || exit 1
ENTRYPOINT ["./gatesentry-bin"]
