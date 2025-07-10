#!/bin/sh
arch=$(uname -m)
chmod +x /usr/local/gatesentry/gatesentry-linux
chmod +x /usr/local/gatesentry/gatesentry-linux-arm64
if [ "$arch" = "aarch64" ]; then
  echo "Running on arm64"
  exec /usr/local/gatesentry/gatesentry-linux-arm64
else
  exec /usr/local/gatesentry/gatesentry-linux
fi
