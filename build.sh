#!/bin/bash

# switch into this directory so go knows what to build
cd $(dirname $0)

export GOOS=linux
export GOARCH=386


# get our dependencies, go will complain since we aren't in GOPATH
# but whatever
go get

# from here on though, things should work without complaining
set -e

go build -o cg-common-resource command.go

# if we are running in concourse than compile.yml will set this far
# copy the artifacts we need to build the docker container to this
# folder so that the next step (docker build) can use them
if [ ! -z "$OUTPUT" ]; then
	cp Dockerfile cg-common-resource out $OUTPUT
fi

exit 0;
