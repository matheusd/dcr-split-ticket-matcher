#!/usr/bin/env bash
set -ex

# The script does automatic checking on a Go package and its sub-packages,
# including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. go vet        (http://golang.org/cmd/vet)
# 3. gosimple      (https://github.com/dominikh/go-simple)
# 4. unconvert     (https://github.com/mdempsky/unconvert)
# 5. ineffassign   (https://github.com/gordonklaus/ineffassign)
# 6. race detector (http://blog.golang.org/race-detector)

# gometalinter (github.com/alecthomas/gometalinter) is used to run each each
# static checker.

# To run on docker on windows, symlink /mnt/c to /c and then execute the script
# from the repo path under /c.  See:
# https://github.com/Microsoft/BashOnWindows/issues/1854
# for more details.

#Default GOVERSION
GOVERSION=${1:-1.11}
REPOOWNER=matheusd
REPO=dcr-split-ticket-matcher
DOCKER_OWNER=matheusd
DOCKER_IMAGE_TAG=dcr-split-tickets

testrepo () {

  # TODO: rewrite this for go.sum
  # Check lockfile
  # TMPFILE=$(mktemp)
  # cp Gopkg.lock $TMPFILE && dep ensure && diff Gopkg.lock $TMPFILE >/dev/null
  # if [ $? != 0 ]; then
  #   echo 'lockfile must be updated with dep ensure'
  #   exit 1
  # fi

  # Check linters
  # (currently disabled)
  # --enable=unconvert \
  #  --enable=varcheck \
  #  --enable=structcheck \
  #  --enable=interfacer \
  #  --enable=megacheck \
  gometalinter \
    --disable-all --vendor --deadline=10m \
    --enable=gofmt \
    --enable=vet \
    --enable=ineffassign \
    --enable=golint \
    --enable=deadcode \
    --enable=vetshadow \
    --enable=goconst \
    ./...
  if [ $? != 0 ]; then
    echo 'gometalinter has some complaints'
    exit 1
  fi

  # Test application install
  go install -i

  if [ $? != 0 ]; then
    echo 'go install failed'
    exit 1
  fi

  # Check tests
  env GORACE='halt_on_error=1' go test ./...
  if [ $? != 0 ]; then
    echo 'go tests failed'
    exit 1
  fi

  echo "------------------------------------------"
  echo "Tests completed successfully!"
}

if [ $GOVERSION == "local" ]; then
    testrepo
    exit
fi

mkdir -p ~/.cache

if [ -f ~/.cache/$DOCKER_IMAGE_TAG.tar ]; then
	# load via cache
	docker load -i ~/.cache/$DOCKER_IMAGE_TAG.tar
	if [ $? != 0 ]; then
		echo 'docker load failed'
		exit 1
	fi
else
	# pull and save image to cache 
	docker pull $DOCKER_OWNER/$DOCKER_IMAGE_TAG
	if [ $? != 0 ]; then
		echo 'docker pull failed'
		exit 1
	fi
	docker save $DOCKER_OWNER/$DOCKER_IMAGE_TAG > ~/.cache/$DOCKER_IMAGE_TAG.tar
	if [ $? != 0 ]; then
		echo 'docker save failed'
		exit 1
	fi
fi

docker run --rm -it -v $(pwd):/src $DOCKER_OWNER/$DOCKER_IMAGE_TAG /bin/bash -c "\
  go get github.com/mattn/go-gtk/gtk && \
  mkdir -p /go/src/github.com/$REPOOWNER/$REPO && \
  rsync -ra --filter=':- .gitignore'  \
  /src/ /go/src/github.com/$REPOOWNER/$REPO/ && \
  cd github.com/$REPOOWNER/$REPO/ && \
  bash run_tests.sh local"
if [ $? != 0 ]; then
	echo 'docker run failed'
	exit 1
fi
