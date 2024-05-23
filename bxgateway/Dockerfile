ARG SG_VERSION

FROM --platform=linux/amd64 golang:1.21-alpine3.18 as builder
COPY .. /tmp/solana-bdn
WORKDIR /tmp/solana-bdn/bxgateway
RUN apk update && apk add libpcap-dev
RUN apk add alpine-sdk
RUN CGO_ENABLED=1 GOWORK=off go build -mod vendor -o gateway ./cmd/gateway/

FROM alpine:latest
ARG SG_VERSION
ENV SG_VERSION=${SG_VERSION}
RUN apk update && apk add libpcap-dev
WORKDIR /root/
COPY --from=builder /tmp/solana-bdn/bxgateway/gateway .
ENTRYPOINT ["./gateway"]