# Pinned to the build host's own platform, not the target one: the build
# stage's output (bundled JS) is architecture-independent, but pnpm
# install + vite build (esbuild - heavily multi-threaded) is exactly the
# kind of long, thread-heavy workload that hangs for hours under QEMU's
# user-mode emulation on a non-native --platform target (observed twice on
# the linux/arm64 leg of the release build). Only the lightweight final
# stage below - a pure-JS `pnpm install --prod`, no native deps - actually
# needs to run under emulation for the target platform.
FROM --platform=$BUILDPLATFORM node:22-alpine AS build
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
