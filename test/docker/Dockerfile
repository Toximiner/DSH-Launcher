ARG BASE=ubuntu:26.04
FROM ${BASE}
ARG NODE_VER
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb dbus polkitd pkexec procps ca-certificates curl xz-utils iproute2 \
      python3 python3-pytest python3-websocket
# Лаунчер из текущего кода, но версией 1.1.0-1 — «старее» релиза на GitHub
# (сценарии обновления лаунчера ставят настоящий последний релиз).
COPY dsh-launcher_1.1.0-1_amd64.deb /tmp/
RUN apt-get install -y /tmp/dsh-launcher_1.1.0-1_amd64.deb && rm /tmp/dsh-launcher_1.1.0-1_amd64.deb
# Node 22 — для сценариев с настоящим dsh (npm, nvm, root-префикс).
COPY node-${NODE_VER}-linux-x64.tar.xz /tmp/
# --no-same-owner: файлы архива Node принадлежат uid 1000; в Debian это и есть
# tester (в Ubuntu 1000 — встроенный ubuntu), и «root-префикс» /opt/noderoot
# оказался бы доступен ему на запись.
RUN mkdir -p /opt/node22 && tar -xJf /tmp/node-${NODE_VER}-linux-x64.tar.xz -C /opt/node22 --strip-components=1 --no-same-owner \
    && rm /tmp/node-${NODE_VER}-linux-x64.tar.xz
RUN useradd -m -s /bin/bash tester
# Старый настоящий dsh: (1) в nvm пользователя — prefix доступен на запись;
# (2) в ~/.local — «другой prefix»; (3) в /opt/noderoot (root) — нужен sudo.
RUN su tester -c "mkdir -p ~/.nvm/versions/node/${NODE_VER} && cp -a /opt/node22/. ~/.nvm/versions/node/${NODE_VER}/ \
      && export PATH=\$HOME/.nvm/versions/node/${NODE_VER}/bin:\$PATH \
      && npm i -g --no-fund --no-audit @deepseek-ai/dsh@0.1.7-rc.2 \
      && npm i -g --no-fund --no-audit --prefix ~/.local @deepseek-ai/dsh@0.1.7-rc.2"
RUN cp -a /opt/node22 /opt/noderoot && PATH=/opt/noderoot/bin:$PATH npm i -g --no-fund --no-audit @deepseek-ai/dsh@0.1.7-rc.2
COPY t /opt/t/
RUN chmod -R a+rX /opt/t && chmod 755 /opt/t/fakes/*.py
