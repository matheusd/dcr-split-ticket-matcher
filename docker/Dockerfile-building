#matheusd-dcr-split-tickets
# This image may be called with the run_tests.sh script included in any of the
# supported go repos.
# ./run_tests.sh 1.10

FROM golang:1.11.1

LABEL description="Decred split tickets builder image"
LABEL version="1.1"
LABEL maintainer "docker-ops@matheusd.org"

ENV TERM linux
ENV USER build

# create user
RUN adduser --disabled-password --gecos ''  $USER

# update base distro & install build tooling
ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && \
    apt-get install -qy rsync git libgtk2.0-dev libglib2.0-dev libgtksourceview2.0-dev

# create directory for build artifacts, adjust user permissions
RUN mkdir /release && \
    chown $USER /release

# create directory to get source from
RUN mkdir /src && \
    chown $USER /src

# switch user
USER $USER
ENV HOME /home/$USER
ENV PATH "${HOME}/bin:${PATH}"

# Download and install gometalinter

WORKDIR /go/src
RUN mkdir -p /home/$USER/bin && \
    wget -O gometalinter.tar.gz https://github.com/alecthomas/gometalinter/releases/download/v2.0.11/gometalinter-2.0.11-linux-amd64.tar.gz && \
    tar -xf gometalinter.tar.gz -C /home/$USER/bin --strip 1
