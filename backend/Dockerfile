FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/widoroku ./cmd/api
FROM alpine:3.21
RUN adduser -D -H appuser
USER appuser
COPY --from=build /out/widoroku /widoroku
EXPOSE 8080
ENTRYPOINT ["/widoroku"]
