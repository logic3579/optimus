# Multi-stage Dockerfile for the optimus-fe SPA.
# Build context MUST be the repo root so deploy/nginx.conf is reachable.

FROM oven/bun:1 AS build
WORKDIR /src
COPY optimus-fe/package.json optimus-fe/bun.lock ./
RUN bun install --frozen-lockfile
COPY optimus-fe/ ./
RUN bun run build

FROM nginx:1.27-alpine
RUN apk add --no-cache wget
COPY --from=build /src/dist /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
