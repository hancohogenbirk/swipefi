# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache upx ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /swipefi ./cmd/swipefi && \
    upx --best --lzma /swipefi

# Stage 3: Scratch runtime (smallest possible image)
FROM scratch
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=backend /swipefi /usr/local/bin/swipefi
EXPOSE 8080
ENTRYPOINT ["swipefi"]
