#!/bin/sh

. ./bin/common.sh

rm -fR dist/release/linux64/stmvotepoolintegrator
mkdir -p dist/release/linux64/stmvotepoolintegrator
mkdir -p dist/archives/v$VERSION

echo "Building integrator (linux64)"
go_build \
  dist/release/linux64/stmvotepoolintegrator/stmvotepoolintegrator \
  ./cmd/stmvotepoolintegrator

ZIPFILE="stmvotepoolintegrator-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE stmvotepoolintegrator

echo "Built integrator binaries $VERSION"
