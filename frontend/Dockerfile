# syntax=docker/dockerfile:1

# ===========================================================================
# BLIPS IFRS9 frontend (blips-web) — multi-stage build
# Next.js 14 standalone output. Dibangun di CI / mesin ber-jaringan.
# ===========================================================================

# --- Stage 1: dependencies ---------------------------------------------------
FROM node:20-alpine AS deps
WORKDIR /app

RUN corepack enable && corepack prepare pnpm@9 --activate

COPY package.json pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile || pnpm install

# --- Stage 2: builder --------------------------------------------------------
FROM node:20-alpine AS builder
WORKDIR /app

RUN corepack enable && corepack prepare pnpm@9 --activate

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Base URL backend API. Variabel NEXT_PUBLIC_* di-inline ke bundle saat `next build`,
# jadi nilainya HARUS tersedia di stage builder (bukan di runtime). Dengan ARG ini satu
# image bisa dibangun ulang per-environment tanpa mengubah kode:
#   docker build --build-arg NEXT_PUBLIC_API_URL=https://api.uat.tugure.example -t blips-web .
# Default tetap http://localhost:8081 agar perilaku dev lokal tidak berubah.
ARG NEXT_PUBLIC_API_URL=http://localhost:8081
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}

ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

# --- Stage 3: runner ---------------------------------------------------------
FROM node:20-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3001
ENV HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs \
  && adduser --system --uid 1001 nextjs

# Standalone output sudah memuat node_modules minimal + server.js
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

USER nextjs

EXPOSE 3001

CMD ["node", "server.js"]
