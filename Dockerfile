# build stage
FROM golang:alpine AS build-env
RUN apk update && apk add curl git
RUN mkdir -p /go/src/github.com/jasonrichardsmith/mwcexample
WORKDIR /go/src/github.com/jasonrichardsmith/mwcexample
COPY main.go .
COPY glide.yaml .
COPY glide.lock .
RUN curl https://glide.sh/get | sh
RUN glide install
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o webhook

FROM alpine:latest

COPY --from=build-env /go/src/github.com/jasonrichardsmith/mwcexample/webhook .
ENTRYPOINT ["/webhook"]
