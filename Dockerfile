FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/linkpulse .

FROM alpine:3.21

RUN adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/linkpulse /usr/local/bin/linkpulse
RUN mkdir -p /app/data && chown -R app:app /app

USER app
ENV ADDR=:8080
ENV LINKPULSE_DATA=/app/data/linkpulse.json
EXPOSE 8080

ENTRYPOINT ["linkpulse"]
