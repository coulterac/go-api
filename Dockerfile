#
# Build container
#
FROM golang:1.13-alpine as build

# use the default value of docker to denote this was built with Docker, this can be overriden
# by specifying a build-arg in the CI process
ARG BUILD=docker

# copy the files into the GOPATH
WORKDIR /app
COPY . .

RUN go env -w GOPRIVATE=github.com/RedVentures
RUN go install -mod=vendor -ldflags "-X main.build=${BUILD}" github.com/coulterac/go-api/cmd/server

#
# App container
#
FROM alpine:latest

# Set work directory
WORKDIR /app/

# Copy in our app
COPY --from=build /go/bin/server /bin/