# Use a smaller base image for the build stage
FROM golang:1.27.1-alpine AS builder

LABEL stage=gobuilder

ARG TARGETARCH
ARG VERSION=unknown
ARG CHANNEL=dev
# UTC RFC 3339 (YYYY-MM-DDTHH:MM:SSZ); empty means the time of the build.
ARG BUILD_TIME=
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}

# Combine apk commands into one to reduce layer size
RUN apk update --no-cache && apk add --no-cache tzdata ca-certificates

WORKDIR /build

# Copy go.mod and go.sum first to take advantage of Docker caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Ship an empty configuration: a locally initialized etc/ppanel.yaml holds the
# JWT secret and database credentials and must never be baked into the image
# (.dockerignore keeps it out of the build context as well).
RUN mkdir -p etc && : > etc/ppanel.yaml

# Build the binary with version and build time. script/ldflags.sh defines the
# injected metadata for every build path (make, this image, the release).
RUN LDFLAGS="$(VERSION="${VERSION}" CHANNEL="${CHANNEL}" BUILD_TIME="${BUILD_TIME}" sh script/ldflags.sh)" && \
    go build -trimpath -ldflags="${LDFLAGS}" -o /app/ppanel main.go

# Final minimal image
FROM scratch

# Copy CA certificates and timezone data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai

ENV TZ=Asia/Shanghai

# Set working directory and copy binary
WORKDIR /app

COPY --from=builder /app/ppanel /app/ppanel
COPY --from=builder /build/etc /app/etc

# Expose the port (optional)
EXPOSE 8080

# Specify entry point
ENTRYPOINT ["/app/ppanel"]
CMD ["run", "--config", "etc/ppanel.yaml"]
