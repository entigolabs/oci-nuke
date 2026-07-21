FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /oci-nuke .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && \
    addgroup -g 1000 oci-nuke && adduser -u 1000 -G oci-nuke -D oci-nuke
COPY --from=build /oci-nuke /usr/local/bin/oci-nuke
USER oci-nuke
ENTRYPOINT ["/usr/local/bin/oci-nuke"]
