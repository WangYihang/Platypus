FROM node:20-alpine AS builder
WORKDIR /app
COPY yarn.lock package.json ./
RUN yarn install
COPY . .
RUN yarn build

FROM node:20-alpine
RUN apk update && apk upgrade
RUN apk add --no-cache dumb-init
WORKDIR /app

COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

EXPOSE 80
ENV HOST=0.0.0.0 NODE_ENV=production PORT=80
CMD ["dumb-init", "node", "server.js"]