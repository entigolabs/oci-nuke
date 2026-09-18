# The build stage always runs on the builder's own architecture and
# cross-compiles to the target, so an arm64 image does not cost a full QEMU
# emulation of the Go toolchain.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
# go.mod replaces github.com/ekristen/libnuke with ./third_party/libnuke, so the
# replace target has to be present before the module cache can be populated.
COPY go.mod go.sum ./
COPY third_party/ ./third_party/
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -o /oci-nuke .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && \
    addgroup -g 1000 oci-nuke && adduser -u 1000 -G oci-nuke -D oci-nuke
COPY --from=build /oci-nuke /usr/local/bin/oci-nuke
USER oci-nuke
ENTRYPOINT ["/usr/local/bin/oci-nuke"]
