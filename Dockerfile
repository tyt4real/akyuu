# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The embedder links against onnxruntime via cgo, so CGO must be enabled.
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/archiver ./cmd/archiver \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/cirno ./cmd/cirno \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/sunny-milk ./cmd/sunny-milk \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/luna-child ./cmd/luna-child \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/star-sapphire ./cmd/star-sapphire

# --- runtime stage -----------------------------------------------------------
# Debian-based: the ONNX Runtime shared library needs glibc (not musl), so
# alpine cannot run the embedder.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates tzdata libgomp1 \
        tesseract-ocr tesseract-ocr-eng tesseract-ocr-jpn tesseract-ocr-chi-sim \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 10001 -d /akyuu akyuu

# Install ONNX Runtime (v1.29.1 - supports C API 29 for onnxruntime_go v1.35.0)
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl \
    && curl -fsSL https://github.com/microsoft/onnxruntime/releases/download/v1.29.1/onnxruntime-linux-x64-1.29.1.tgz \
        | tar -xz -C /usr/local \
    && mv /usr/local/onnxruntime-linux-x64-1.29.1 /usr/local/onnxruntime \
    && mkdir -p /usr/local/lib/onnxruntime \
    && ln -s /usr/local/onnxruntime/lib/libonnxruntime.so /usr/local/lib/onnxruntime/libonnxruntime.so \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get purge -y curl

WORKDIR /akyuu

# libonnxruntime.so must sit where onnxruntime_go can dlopen it.
ENV LD_LIBRARY_PATH=/usr/local/lib/onnxruntime
ENV ORT_LIBRARY_PATH=/usr/local/lib/onnxruntime/libonnxruntime.so

COPY --from=build /out/archiver /usr/local/bin/archiver
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/cirno /usr/local/bin/cirno
COPY --from=build /out/sunny-milk /usr/local/bin/sunny-milk
COPY --from=build /out/luna-child /usr/local/bin/luna-child
COPY --from=build /out/star-sapphire /usr/local/bin/star-sapphire

# Default images are built for the archiver; the api binary ships alongside it
# (docker run <image> api -listen :8080). The ONNX model files and
# libonnxruntime.so are mounted at runtime — see docs/semantic-search.md — and
# the pipeline is enabled via the embeddings section of config.
# 
# Workers:
#   docker run <image> cirno -config /akyuu/config/config.yaml
#   docker run <image> sunny-milk -config /akyuu/config/config.yaml
#   docker run <image> luna-child -config /akyuu/config/config.yaml
#   docker run <image> star-sapphire -config /akyuu/config/config.yaml
# 
# Note: whisper.cpp binary is NOT included (not in Debian repos).
# Install manually or build from source: https://github.com/ggerganov/whisper.cpp
COPY config/config.yaml.example ./config/config.yaml
COPY config/sites ./config/sites
RUN touch ./config/csam_hashes.txt \
    && mkdir -p storage \
    && chown -R akyuu:akyuu /akyuu

USER akyuu
VOLUME ["/akyuu/storage", "/akyuu/config"]
ENTRYPOINT ["archiver"]
CMD ["-config", "/akyuu/config/config.yaml"]