#!/bin/sh
# Substitute only the known environment variables in the mounted config template,
# write the result to a writable location, then exec mediamtx.
# We explicitly list variables to avoid clobbering MediaMTX-internal placeholders
# like $G1 (path capture groups) or $MTX_* variables.
set -e
envsubst '$PUBLISH_SECRET $INTERNAL_SECRET $VIEWER_SECRET $WEBRTC_EXTERNAL_HOST' \
  < /mediamtx.yml > /tmp/mediamtx.yml
exec /mediamtx /tmp/mediamtx.yml
