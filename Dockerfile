FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS app
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /nonton ./cmd/nonton

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=app /nonton /nonton
ENV NONTON_DATA=/data NONTON_ADDR=:8787
VOLUME /data
EXPOSE 8787
ENTRYPOINT ["/nonton"]
