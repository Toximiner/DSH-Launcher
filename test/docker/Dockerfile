ARG BASE=ubuntu:26.04
FROM ${BASE}
ARG NODE_VER
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb weston xdotool x11-xkb-utils dbus polkitd pkexec procps ca-certificates curl xz-utils iproute2 \
      python3 python3-pytest python3-websocket python3-gi libnotify4 dbus-session-bus-common
# Node 22 — for the scenarios with the real dsh (npm, nvm, root prefix).
COPY node-${NODE_VER}-linux-x64.tar.xz /tmp/
# --no-same-owner: the Node archive files belong to uid 1000; in Debian that
# is tester (in Ubuntu 1000 is the built-in ubuntu), and the "root prefix"
# /opt/noderoot would end up writable by him.
RUN mkdir -p /opt/node22 && tar -xJf /tmp/node-${NODE_VER}-linux-x64.tar.xz -C /opt/node22 --strip-components=1 --no-same-owner \
    && rm /tmp/node-${NODE_VER}-linux-x64.tar.xz
RUN useradd -m -s /bin/bash tester
# The old real dsh: (1) in the user's nvm — a writable prefix;
# (2) in ~/.local — "another prefix"; (3) in /opt/noderoot (root) — needs sudo.
RUN su tester -c "mkdir -p ~/.nvm/versions/node/${NODE_VER} && cp -a /opt/node22/. ~/.nvm/versions/node/${NODE_VER}/ \
      && export PATH=\$HOME/.nvm/versions/node/${NODE_VER}/bin:\$PATH \
      && npm i -g --no-fund --no-audit @deepseek-ai/dsh@0.1.7-rc.2 \
      && npm i -g --no-fund --no-audit --prefix ~/.local @deepseek-ai/dsh@0.1.7-rc.2"
RUN cp -a /opt/node22 /opt/noderoot && PATH=/opt/noderoot/bin:$PATH npm i -g --no-fund --no-audit @deepseek-ai/dsh@0.1.7-rc.2
# The launcher package and tests change on every run — so they go last: the
# layers above (apt, Node, old dsh) come from the build cache. apt-get update
# is here too: the package lists in a cached layer may have gone stale.
# The launcher is from the current code but with version 1.1.0-1 — "older"
# than the fake GitHub's latest release (2.0.0-1; its .deb is wired in by
# run.sh into /opt/github).
COPY dsh-launcher_1.1.0-1_amd64.deb /tmp/
RUN apt-get update && apt-get install -y /tmp/dsh-launcher_1.1.0-1_amd64.deb && rm /tmp/dsh-launcher_1.1.0-1_amd64.deb
COPY t /opt/t/
RUN chmod -R a+rX /opt/t && chmod 755 /opt/t/fakes/*.py
