#!/bin/bash

# WARNING. This is a crude and destructive deploy script.
# It makes no guarantees of uptime or stability.

# build new octadground dist
cd ~/octadground && git pull && yarn run dist

# copy dist into lioctad res directory
cd ~/lioctad/src && cp ~/octadground/dist/octadground.js ./static/res

# remove and prune old stack
docker stack rm lio-prod
docker image prune -f

# rebuild lioctad container
docker build -t lioctad:latest -f Dockerfile .

# remove octadground dist so we don't have local changes
rm -rf ~/lioctad/src/static/res/octadground.js

# deploy and scale service down
cd ../deploy && docker stack deploy -c lioctad-stack.yml lio-prod
docker service scale lio-prod_lioctad=1

# watch the magic
watch docker service ls