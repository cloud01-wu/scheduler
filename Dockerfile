# Build Stage
FROM golang:1.21.1 as builder
ARG GIT_SERVER
ARG GIT_USERNAME
ARG GIT_PASSWORD
RUN touch ~/.netrc && echo "machine "$GIT_SERVER" login "$GIT_USERNAME" password "$GIT_PASSWORD > ~/.netrc
WORKDIR /src
ADD ./src /src
RUN cd /src && make docker

# Final Stage
FROM ubuntu:22.04
RUN apt-get update
RUN DEBIAN_FRONTEND=noninteractive apt-get install -yq ca-certificates
RUN rm -Rf /var/lib/apt/lists/*
COPY --from=builder /src/bin/docker/scheduler /usr/local/bin/.
COPY ./db /opt/db
CMD ["/usr/local/bin/scheduler"]
