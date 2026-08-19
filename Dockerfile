# Multi-stage, pure-Go build: no CGO, so the runtime image needs no libc beyond
# what a static binary uses. modernc.org/sqlite is CGO-free by design.
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build \
        -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
        -o /out/situs ./cmd/situs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/situs /usr/local/bin/situs
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/situs"]
CMD ["serve"]
