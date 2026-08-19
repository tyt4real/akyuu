# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/archiver ./cmd/archiver

# --- runtime stage -----------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /akyuu akyuu

WORKDIR /akyuu

COPY --from=build /out/archiver /usr/local/bin/archiver
# Sane defaults; mount your own config over /akyuu/config at runtime.
COPY config/config.yaml.example ./config/config.yaml
COPY config/sites ./config/sites
RUN touch ./config/csam_hashes.txt \
    && mkdir -p storage \
    && chown -R akyuu:akyuu /akyuu

USER akyuu
VOLUME ["/akyuu/storage", "/akyuu/config"]
ENTRYPOINT ["archiver"]
CMD ["-config", "/akyuu/config/config.yaml"]