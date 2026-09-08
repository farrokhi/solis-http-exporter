FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
        -o /solis_http_exporter ./cmd/solis_http_exporter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /solis_http_exporter /usr/bin/solis_http_exporter

EXPOSE 9613
USER nonroot:nonroot
ENTRYPOINT ["/usr/bin/solis_http_exporter"]
CMD ["--config.file=/etc/solis-http-exporter/config.yml"]
