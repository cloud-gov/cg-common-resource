#!/bin/bash

GOOS=linux go build -o in command.go
cp in check

docker build --no-cache -t $DOCKER_USER/cg-common-resource .
docker push $DOCKER_USER/cg-common-resource

