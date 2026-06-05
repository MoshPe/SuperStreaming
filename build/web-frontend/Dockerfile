FROM node:20-alpine AS builder
WORKDIR /app
COPY services/web-frontend/package*.json ./
RUN npm ci
COPY services/web-frontend/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY services/web-frontend/nginx.conf /etc/nginx/conf.d/default.conf
# Keep template; entrypoint substitutes env vars at runtime
COPY services/web-frontend/public/env-config.js /usr/share/nginx/html/env-config.js.template
CMD ["sh", "-c", "envsubst < /usr/share/nginx/html/env-config.js.template > /usr/share/nginx/html/env-config.js && nginx -g 'daemon off;'"]
