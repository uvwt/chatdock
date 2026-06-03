FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -buildvcs=false -o /out/chatdock ./cmd/chatdock

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -u 10001 -g nogroup chatdock
COPY --from=build /out/chatdock /app/chatdock
COPY web /app/web
ENV CHATDOCK_ADDR=:8720
ENV CHATDOCK_DATA=/data
ENV CHATDOCK_WEB=/app/web
RUN mkdir -p /data && chown -R chatdock:nogroup /data /app
USER chatdock
VOLUME ["/data"]
EXPOSE 8720
ENTRYPOINT ["/app/chatdock"]
