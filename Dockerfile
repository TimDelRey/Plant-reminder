# 1) сборка фронтенда (типы API генерируются из internal/api/openapi.yaml)
FROM node:22-alpine AS web
WORKDIR /src
COPY internal/api/openapi.yaml internal/api/openapi.yaml
COPY web/package.json web/package-lock.json web/
RUN cd web && npm ci
COPY web web
RUN cd web && npm run build

# 2) сборка Go-бинарника со встроенным фронтендом (go:embed)
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

# 3) минимальный рантайм: один бинарник, один порт
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
COPY --from=build /server /server
USER app
EXPOSE 8080
ENTRYPOINT ["/server"]
