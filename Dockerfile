# https://cirrus-ci.com/github/btwiuse/k0s

FROM golang:alpine AS builder

RUN go install github.com/btwiuse/pd@latest

FROM alpine

COPY --from=builder /go/bin/pd /bin/

ENTRYPOINT ["pd"]
