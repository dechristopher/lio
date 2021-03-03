#!/bin/bash

# WARNING. This is a crude and destructive deploy script.
# It makes no guarantees of uptime or stability.

# remove and prune old stack
docker stack rm lio-prod
docker image prune -f

# rebuild lioctad container
cd ../src && docker build -t lioctad:latest -f Dockerfile .

# deploy and scale service down
cd ../deploy && docker stack deploy -c lioctad-stack.yml lio-prod
docker service scale lio-prod_lioctad=1

# watch the magic
watch docker service ls