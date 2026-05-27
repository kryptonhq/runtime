# syntax=docker/dockerfile:1.7
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/adk-agent ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/adk-agent /adk-agent
USER nonroot
EXPOSE 8001
ENTRYPOINT ["/adk-agent"]
