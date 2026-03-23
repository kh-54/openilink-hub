# --- Build frontend ---
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Build backend ---
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache git gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=1 go build -o /openilink-hub .

# --- Runtime ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=backend /openilink-hub /usr/local/bin/openilink-hub
EXPOSE 9800
ENTRYPOINT ["openilink-hub"]
CMD ["-listen", "0.0.0.0:9800"]
