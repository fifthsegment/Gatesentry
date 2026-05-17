FROM node:20-alpine AS ui-builder
WORKDIR /src/ui
COPY ui/package.json ui/yarn.lock ui/.yarnrc.yml ./
RUN yarn install --frozen-lockfile
COPY ui/ .
RUN yarn build

FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache bash
WORKDIR /src
COPY . .
RUN go mod download
COPY --from=ui-builder /src/ui/dist/ /src/application/webserver/frontend/files/
RUN mv /src/application/webserver/frontend/files/fs/* /src/application/webserver/frontend/files/ 2>/dev/null || true
RUN OUTPUT=/gatesentry-bin ./build.sh --no-ui

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /usr/local/gatesentry
COPY --from=go-builder /gatesentry-bin ./
RUN mkdir -p /usr/local/gatesentry/gatesentry
EXPOSE 53/udp 53/tcp 10413 10786
ENTRYPOINT ["./gatesentry-bin"]
