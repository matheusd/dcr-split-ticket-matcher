#!/usr/bin/env bash

set -ex

DOCKER_OWNER=matheusd
DOCKER_IMAGE_TAG=dcr-split-tickets

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
  rsync -ra --filter=':- .gitignore'  \
  /src/ /src/ && \
  cd /src/ && \
  bash tests.sh"
if [ $? != 0 ]; then
	echo 'docker run failed'
	exit 1
fi
