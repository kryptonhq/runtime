# syntax=docker/dockerfile:1.7
FROM golang:1.25 AS build
WORKDIR /src
COPY examples/mcp/go/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp-hello ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mcp-hello /mcp-hello
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/mcp-hello"]
