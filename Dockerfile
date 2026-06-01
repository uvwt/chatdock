FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/chatdock ./cmd/chatdock

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/chatdock /app/chatdock
COPY web /app/web
ENV CHATDOCK_ADDR=:8720
ENV CHATDOCK_DATA=/data
ENV CHATDOCK_WEB=/app/web
VOLUME ["/data"]
EXPOSE 8720
ENTRYPOINT ["/app/chatdock"]
