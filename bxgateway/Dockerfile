ARG SG_VERSION

FROM golang:1.24.2-bullseye AS builder
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libpcap-dev pkg-config ca-certificates && rm -rf /var/lib/apt/lists/*

ARG WORKDIR=/tmp/solana-bdn
ARG FD_LIB_PATH=bxgateway/internal/firedancer/lib
WORKDIR ${WORKDIR}
COPY . .
RUN cd ${FD_LIB_PATH} && make clean && make
RUN cd bxgateway && CGO_ENABLED=1 CGO_CFLAGS="-I${WORKDIR}/${FD_LIB_PATH}" CGO_LDFLAGS="-L${WORKDIR}/${FD_LIB_PATH} -lfdshredmonitor" GOWORK=off go build -ldflags="-s -w" -o gateway ./cmd/gateway/

# collect runtime libraries in builder stage
RUN mkdir -p /tmp/libs && for lib in $(ldd bxgateway/gateway | awk '/=>/ {print $3}' | grep -vE 'libc.so|ld-linux|libm.so|libpthread.so|librt.so'); do cp -v "$lib" /tmp/libs/; done

FROM debian:bookworm-slim
ARG SG_VERSION
ENV SG_VERSION=${SG_VERSION}
RUN apt-get update \
 && apt-get install -y --no-install-recommends libpcap0.8 libdbus-1-3 ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /
# copy the gateway binary from builder
COPY --from=builder /tmp/solana-bdn/bxgateway/gateway /gateway
ENTRYPOINT ["/gateway"]
