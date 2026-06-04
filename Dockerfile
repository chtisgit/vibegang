FROM golang:latest

# Install necessary tools
RUN apt-get update && apt-get install -y \
    git \
    make \
    openssh-client \
    bash \
    jq \
    curl \
    postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Setup SSH config to avoid host key verification issues for known git providers
RUN mkdir -p /root/.ssh && \
    chmod 700 /root/.ssh && \
    ssh-keyscan github.com >> /root/.ssh/known_hosts && \
    ssh-keyscan gitlab.com >> /root/.ssh/known_hosts

# Provide the agent binary
WORKDIR /app
COPY vibegang-agent /usr/local/bin/vibegang-agent
RUN chmod +x /usr/local/bin/vibegang-agent

WORKDIR /workspace

CMD ["vibegang-agent"]
