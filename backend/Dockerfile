FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sandbox-api ./cmd/api
FROM alpine:3.22
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=build /out/sandbox-api /usr/local/bin/sandbox-api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sandbox-api"]
