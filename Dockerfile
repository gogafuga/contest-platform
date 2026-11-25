FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# Requests use the browser's current origin; never bake a laptop address into JS.
RUN VITE_API_URL= npm run build

FROM golang:1.26.10-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /contest01 .

FROM alpine:3.23
RUN addgroup -S contest && adduser -S -G contest contest
WORKDIR /app
COPY --from=backend /contest01 ./contest01
COPY backend/config.toml ./config.toml
COPY --from=frontend /src/frontend/dist ./web
ENV PORT=8080 STATIC_DIR=/app/web GIN_MODE=release
USER contest
EXPOSE 8080
CMD ["./contest01"]
