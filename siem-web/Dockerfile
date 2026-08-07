FROM node:22-alpine AS build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
RUN corepack prepare pnpm@9 --activate && pnpm install --frozen-lockfile
COPY . .
RUN pnpm run build

FROM node:22-alpine
RUN apk add --no-cache ca-certificates wget && corepack enable
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
RUN corepack prepare pnpm@9 --activate && pnpm install --frozen-lockfile --prod
COPY --from=build /src/build ./build

ENV NODE_ENV=production
ENV PORT=8080
ENV HOST=0.0.0.0
EXPOSE 8080
ENTRYPOINT ["node", "build/index.js"]
