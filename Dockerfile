# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The embedder links against onnxruntime via cgo, so CGO must be enabled.
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/archiver ./cmd/archiver \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime stage -----------------------------------------------------------
# Debian-based: the ONNX Runtime shared library needs glibc (not musl), so
# alpine cannot run the embedder.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates tzdata libgomp1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 10001 -d /akyuu akyuu

WORKDIR /akyuu

# libonnxruntime.so must sit where onnxruntime_go can dlopen it.
ENV LD_LIBRARY_PATH=/usr/local/lib/onnxruntime
ENV ORT_LIBRARY_PATH=/usr/local/lib/onnxruntime/libonnxruntime.so

COPY --from=build /out/archiver /usr/local/bin/archiver
COPY --from=build /out/api /usr/local/bin/api

# Default images are built for the archiver; the api binary ships alongside it
# (docker run <image> api -listen :8080). The ONNX model files and
# libonnxruntime.so are mounted at runtime — see docs/semantic-search.md — and
# the pipeline is enabled via the embeddings section of config.
COPY config/config.yaml.example ./config/config.yaml
COPY config/sites ./config/sites
RUN touch ./config/csam_hashes.txt \
    && mkdir -p storage \
    && chown -R akyuu:akyuu /akyuu

USER akyuu
VOLUME ["/akyuu/storage", "/akyuu/config"]
ENTRYPOINT ["archiver"]
CMD ["-config", "/akyuu/config/config.yaml"]