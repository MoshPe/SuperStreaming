FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY services/api-server/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /api-server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /api-server /api-server
ENTRYPOINT ["/api-server"]
