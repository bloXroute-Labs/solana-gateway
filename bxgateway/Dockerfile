ARG SG_VERSION

FROM --platform=linux/amd64 golang:1.24.2-alpine as builder
RUN apk update && apk add libpcap-dev
RUN apk add alpine-sdk

ARG WORKDIR=/tmp/solana-bdn
ARG FD_LIB_PATH=bxgateway/internal/firedancer/lib
WORKDIR ${WORKDIR}
COPY . .
RUN cd ${FD_LIB_PATH} && make clean && make
RUN cd bxgateway && CGO_ENABLED=1 CGO_CFLAGS="-I${WORKDIR}/${FD_LIB_PATH}" CGO_LDFLAGS="-L${WORKDIR}/${FD_LIB_PATH} -lfdshredmonitor" GOWORK=off go build -o gateway ./cmd/gateway/

FROM alpine:latest
ARG SG_VERSION
ARG WORKDIR=/tmp/solana-bdn
ARG FD_LIB_PATH=bxgateway/internal/firedancer/lib
ENV SG_VERSION=${SG_VERSION}
RUN apk update && apk add libpcap-dev
WORKDIR /root/
COPY --from=builder ${WORKDIR}/bxgateway/gateway .
COPY --from=builder ${WORKDIR}/${FD_LIB_PATH}/libfdshredmonitor.a .
ENV LD_LIBRARY_PATH=/root
ENTRYPOINT ["./gateway"]
