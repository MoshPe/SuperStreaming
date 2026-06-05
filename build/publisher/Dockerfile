FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY services/publisher/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /publisher .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg
COPY --from=builder /publisher /publisher
ENTRYPOINT ["/publisher"]
