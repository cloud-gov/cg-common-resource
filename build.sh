#!/bin/bash

GOOS=linux go build in.go

docker build --no-cache -t $DOCKER_USER/cg-common-resource .
docker push $DOCKER_USER/cg-common-resource

