#!/bin/sh

VERSION=`grep -oP "const Version = \"\K[^\"]+(?=\")" pkg/version.go`

rm -fR dist/release/linux64/stmvotepoolintegrator
mkdir -p dist/release/linux64/stmvotepoolintegrator
mkdir -p dist/archives/v$VERSION

echo "Building integrator binaries $VERSION..."

echo "Building integrator (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/stmvotepoolintegrator/stmvotepoolintegrator \
    cmd/stmvotepoolintegrator/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

ZIPFILE="stmvotepoolintegrator-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE stmvotepoolintegrator

echo "Built integrator binaries $VERSION"
