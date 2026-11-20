FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# .git is not in the build context, so nothing is stamped unless passed in.
ARG VERSION=
ARG COMMIT=
RUN CGO_ENABLED=0 go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
        -o /solis_http_exporter ./cmd/solis_http_exporter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /solis_http_exporter /usr/bin/solis_http_exporter

EXPOSE 9613
USER nonroot:nonroot
ENTRYPOINT ["/usr/bin/solis_http_exporter"]
CMD ["--config.file=/etc/solis-http-exporter/config.yml"]
