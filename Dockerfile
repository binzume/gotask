FROM ubuntu:22.04

# Install tools
RUN apt-get update && \
    apt-get install -y zip curl jq && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

EXPOSE 8080
COPY gotask /gotask
ENTRYPOINT ["/gotask"]

