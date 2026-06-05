# Wraps the official MediaMTX binary with an envsubst entrypoint so that
# $VAR placeholders in mediamtx.yml are expanded from environment variables
# before the server starts.  Alpine provides both sh and envsubst (via gettext).
FROM alpine:3.19 AS base
RUN apk add --no-cache gettext

# Copy the static mediamtx binary from the official image
COPY --from=bluenviron/mediamtx:latest /mediamtx /mediamtx

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
