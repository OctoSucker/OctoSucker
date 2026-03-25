FROM python:3.11-slim-bookworm

ENV DEBIAN_FRONTEND=noninteractive

# deb.debian.org sometimes returns 500 / truncated bodies over plain HTTP during builds; HTTPS + retries helps.
RUN set -eux; \
  printf '%s\n' \
    'Acquire::Retries "10";' \
    'Acquire::http::Timeout "120";' \
    'Acquire::https::Timeout "120";' \
    'Acquire::http::Pipeline-Depth "0";' \
    > /etc/apt/apt.conf.d/99-retry; \
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
    if [ -f "$f" ]; then sed -i 's|http://deb.debian.org|https://deb.debian.org|g' "$f"; fi; \
  done; \
  success=0; \
  for attempt in 1 2 3 4 5; do \
    if apt-get update; then success=1; break; fi; \
    echo "apt-get update failed (attempt $attempt), retrying..."; \
    sleep $((attempt * 5)); \
  done; \
  [ "$success" -eq 1 ]; \
  apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    git \
    tini \
  && rm -rf /var/lib/apt/lists/*

# Optional browser automation CLI available inside sandbox container.
RUN pip install --no-cache-dir browser-use

WORKDIR /workspace/0

ENTRYPOINT ["tini", "--"]
CMD ["bash", "-lc", "sleep infinity"]
