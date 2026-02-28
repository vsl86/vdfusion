# Stage 1: Build Vue Frontend
FROM node:18-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go Backend
FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache build-base pkgconfig ffmpeg-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Ensure frontend/dist exists for embedding
COPY --from=frontend-builder /app/dist ./frontend/dist
RUN go build -o vdfusion .

# Stage 3: Runtime
FROM alpine:3.23
RUN apk update && apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=go-builder /app/vdfusion /usr/local/bin/vdfusion
RUN mkdir -p /app/storage
ENV VDF_DB_PATH=/app/storage/vdfusion.db
ENV VDF_SERVER_ADDR=:8080
EXPOSE 8080
# Run in server mode by default
CMD ["vdfusion", "--server"]
