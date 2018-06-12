#!/bin/sh

VERSION=`grep -oP "const Version = \"\K[^\"]+(?=\")" pkg/version.go`

rm -fR dist/release/linux64/dcrstmd
mkdir -p dist/release/linux64/dcrstmd
mkdir -p dist/archives/v$VERSION

echo "Building service binaries $VERSION..."

echo "Building service (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/dcrstmd/dcrstmd \
    cmd/dcrstmd/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

cp samples/dcrstmd.conf dist/release/linux64/dcrstmd
cp docs/release-readme.md dist/release/linux64/dcrstmd/README.md

ZIPFILE="dcrstmd-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE dcrstmd

echo "Built service binaries $VERSION"
