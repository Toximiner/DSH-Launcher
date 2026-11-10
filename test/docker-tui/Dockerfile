ARG BASE=ubuntu:26.04
FROM ${BASE}
ARG NODE_VER=24.22.1
ARG DSH_VER=0.2.2-rc.1
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates xz-utils python3 python3-pytest python3-pyte procps \
    && rm -rf /var/lib/apt/lists/*

# Wrap the verified official Node archive in a test-only Debian package. This
# supplies a real Node runtime and lets apt validate the TUI's Depends field
# even on distributions whose default nodejs package is older than 22.
# Node 24 is required by the pinned dsh; the separate TUI unit job uses Node 22.
COPY node-v${NODE_VER}-linux-x64.tar.xz /tmp/node.tar.xz
RUN mkdir -p /tmp/node/DEBIAN /tmp/node/usr/lib/tui-test-node /tmp/node/usr/bin \
    && tar -xJf /tmp/node.tar.xz -C /tmp/node/usr/lib/tui-test-node --strip-components=1 --no-same-owner \
    && for bin in node npm npx; do ln -s ../lib/tui-test-node/bin/$bin /tmp/node/usr/bin/$bin; done \
    && printf 'Package: nodejs\nVersion: %s\nArchitecture: amd64\nMaintainer: TUI tests <tests@example.invalid>\nDepends: libc6 (>= 2.28), libstdc++6, libgcc-s1\nDescription: Official Node runtime packaged for TUI integration tests\n' "$NODE_VER" > /tmp/node/DEBIAN/control \
    && chmod 755 /tmp/node/DEBIAN \
    && dpkg-deb --build --root-owner-group /tmp/node /tmp/nodejs.deb \
    && apt-get update && apt-get install -y --no-install-recommends /tmp/nodejs.deb \
    && rm -rf /tmp/node /tmp/node.tar.xz /tmp/nodejs.deb /var/lib/apt/lists/*

# Pin the real backend; model requests are never made by the smoke scenario.
RUN npm install -g --prefix /opt/dsh --ignore-scripts --no-audit --no-fund @deepseek-ai/dsh@${DSH_VER}
COPY launcher.deb /tmp/launcher.deb
RUN apt-get update && apt-get install -y --no-install-recommends /tmp/launcher.deb \
    && rm -f /tmp/launcher.deb && rm -rf /var/lib/apt/lists/*
COPY tests /opt/tui-tests
RUN useradd --create-home --shell /bin/sh tester \
    && chmod -R a+rX /opt/tui-tests && chmod 755 /opt/tui-tests/fakes/dsh.js
ENV HOME=/home/tester TERM=xterm-256color \
    TUI_TEST_BIN=/usr/bin/dsh-launcher-tui \
    TUI_TEST_REAL_DSH=/opt/dsh/bin/dsh \
    TUI_TEST_CONTAINER=1 \
    TUI_TEST_RESULTS=/results
USER tester
WORKDIR /home/tester
CMD ["python3", "-m", "pytest", "-p", "no:cacheprovider", "-v", "--tb=short", "/opt/tui-tests", "--junitxml=/results/junit.xml"]
