# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build flacalyzer from source (always latest main)
FROM alpine:3.21 AS flacalyzer
RUN apk add --no-cache git flac-dev curl xz musl-dev
RUN curl -L https://ziglang.org/download/0.15.2/zig-x86_64-linux-0.15.2.tar.xz | tar -xJ -C /usr/local && \
    ln -s /usr/local/zig-x86_64-linux-0.15.2/zig /usr/local/bin/zig
WORKDIR /build
RUN git clone --depth 1 https://github.com/hancohogenbirk/flacalyzer.git .
RUN zig build -Doptimize=ReleaseFast

# Stage 3: Build Go binary with embedded frontend
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache upx ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /swipefi ./cmd/swipefi && \
    upx --best --lzma /swipefi

# Stage 4: Alpine runtime (needed for flacalyzer subprocess + libFLAC)
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata flac
COPY --from=backend /swipefi /usr/local/bin/swipefi
COPY --from=flacalyzer /build/zig-out/bin/flacalyzer /usr/local/bin/flacalyzer
EXPOSE 8080
ENTRYPOINT ["swipefi"]
