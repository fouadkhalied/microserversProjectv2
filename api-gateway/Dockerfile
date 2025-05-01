# Step 1: Build TS
FROM node:20-bullseye AS builder

WORKDIR /app
COPY package*.json ./
RUN npm install

COPY tsconfig.json ./
COPY src ./src

RUN npx tsc

# Step 2: Run JS
FROM node:20-bullseye

WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY package*.json ./
RUN npm install --omit=dev

EXPOSE 3000
CMD ["node", "dist/server.js"]
