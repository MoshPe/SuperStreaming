FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY services/recording-uploader/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /recording-uploader .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /recording-uploader /recording-uploader
ENTRYPOINT ["/recording-uploader"]
