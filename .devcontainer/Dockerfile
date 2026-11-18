FROM golang:1.27.3-bookworm AS go
FROM node:24.22.1-bookworm
COPY --from=go /usr/local/go /usr/local/go
RUN apt-get update && apt-get install -y --no-install-recommends python3 ca-certificates git \
    && rm -rf /var/lib/apt/lists/*
ENV PATH="/usr/local/go/bin:${PATH}"
USER node
