FROM node:20-alpine AS builder
WORKDIR /app
COPY yarn.lock package.json ./
RUN yarn install
COPY . .
RUN yarn build
ENTRYPOINT [ "yarn", "start" ]
