FROM ubuntu:latest
USER root
RUN apt-get update && apt-get install -y net-tools dnsutils && rm -rf /var/lib/apt/lists/*
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /usr/local/gatesentry
COPY gatesentry-linux /usr/local/gatesentry
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/gatesentry/gatesentry-linux
EXPOSE 80 53 53/UDP 10413 10786
ENTRYPOINT ["/entrypoint.sh"]
