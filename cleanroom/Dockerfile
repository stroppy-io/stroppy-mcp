FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV STROPPY_BIN=/usr/local/bin/stroppy

# PostgreSQL + Node.js + basics
# PostgreSQL is bundled for demo convenience — a real setup would point at an external DB.
RUN apt-get update && apt-get install -y --no-install-recommends \
    postgresql postgresql-client \
    nodejs npm \
    curl ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

# Claude Code
RUN npm install -g @anthropic-ai/claude-code

# Stroppy: prebuilt release
RUN curl -L https://github.com/stroppy-io/stroppy/releases/latest/download/stroppy_linux_amd64.tar.gz | tar xz -C /usr/local/bin/

# stroppy-mcp: copy pre-built binary (built outside Docker for speed)
COPY stroppy-mcp /usr/local/bin/stroppy-mcp

# PostgreSQL: configure for TCP access with password auth
RUN sed -i "s/#listen_addresses = 'localhost'/listen_addresses = '*'/" /etc/postgresql/16/main/postgresql.conf && \
    echo "host all all 0.0.0.0/0 md5" >> /etc/postgresql/16/main/pg_hba.conf

# Claude Code config: pre-wired so MCP server is available immediately
RUN mkdir -p /root/.claude && \
    echo '{}' > /root/.claude/settings.json

# Workspace
RUN mkdir -p /workspace

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

WORKDIR /workspace
EXPOSE 5432

ENTRYPOINT ["/entrypoint.sh"]
CMD ["bash"]
