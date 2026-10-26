# build the Go server
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /telemetrypulse .

# small runtime image with redis next to the app
FROM alpine:3.22
RUN apk add --no-cache redis ca-certificates
WORKDIR /app
COPY --from=build /telemetrypulse ./telemetrypulse
COPY frontend/ ./frontend/
COPY start.sh ./start.sh
ENV STATIC_DIR=/app/frontend
EXPOSE 8080
CMD ["sh", "./start.sh"]
