FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm

# Install libpcap development libraries
RUN apt-get update && apt-get install -y \
    openjdk-17-jdk \
    graphviz \
    libpcap-dev \
    && rm -rf /var/lib/apt/lists/*