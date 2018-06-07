# Docker Images

This was entirely based on the [decred docker
images](https://github.com/decred/dcrdocker). All credit for things working goes
to them. All blame for things breaking are my own. :)

## Cheatsheet

Update image:

```
DOCKER_IMAGE_TAG=dcr-split-tickets
docker build -t $DOCKER_IMAGE_TAG -f Dockerfile-building .
docker tag $DOCKER_IMAGE_TAG matheusd/$DOCKER_IMAGE_TAG
docker push matheusd/$DOCKER_IMAGE_TAG
```
