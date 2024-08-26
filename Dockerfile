FROM golang:alpine as builder
ARG TYPE
ENV GOBIN=/go/bin
ENV GOPATH=/go/src
RUN mkdir /build
WORKDIR /build
RUN apk add --upgrade git
RUN go version
# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

ADD . /build/
RUN echo $TYPE
RUN cd /build/$TYPE; go build -o main .

FROM alpine
ARG TYPE
LABEL maintainer="Chris Blum <cblum@redhat.com>"

LABEL org.label-schema.build-date=$BUILD_DATE \
    org.label-schema.name="goroom-$TYPE" \
    org.label-schema.vcs-ref=$VCS_REF \
    org.label-schema.vcs-url="https://github.com/mulbc/gosbench" \
    org.label-schema.schema-version="1.0"

RUN adduser -S -D -H -h /app appuser
USER appuser
COPY --from=builder /build/$TYPE/main /app/
WORKDIR /app
ENTRYPOINT ["./main"]