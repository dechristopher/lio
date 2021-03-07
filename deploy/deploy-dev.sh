#!/bin/bash

# WARNING. This is a crude and destructive deploy script.
# It makes no guarantees of uptime or stability.

export COMMIT_HASH=${git rev-parse --short HEAD}

# build new octadground dist
cd ~/octadground && git pull && yarn run dist

# copy dist into lioctad res directory
cd ~/lioctad/src && cp ~/octadground/dist/octadground.js ./static

# rebuild lioctad container
docker build -t lioctad:latest -t lioctad:${COMMIT_HASH} -f Dockerfile .

# remove octadground dist so we don't have local changes
rm -rf ~/lioctad/src/static/res/octadground.js

# update stack with new container
cd ../deploy && docker stack deploy -c lio-dev-stack.yml lio-dev

# watch the magic
watch docker service ls