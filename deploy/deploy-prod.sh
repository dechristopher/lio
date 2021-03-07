#!/bin/bash

# WARNING. This is a crude and destructive deploy script.
# It makes no guarantees of uptime or stability.

export COMMIT_HASH=$(git rev-parse --short HEAD)

# WARNING. Make sure to deploy the dev environment first.
# This script assumes the latest commit container has been
# built beforehand in the dev deploy script.

# update stack with new container
cd ../deploy && docker stack deploy -c lio-prod-stack.yml lio-prod

# watch the magic
watch docker service ls