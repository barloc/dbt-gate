# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dbt-gate .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/dbt-gate /app/dbt-gate
COPY config.example.yaml /app/config.example.yaml
EXPOSE 8080 8090
USER nonroot:nonroot
ENTRYPOINT ["/app/dbt-gate"]
CMD ["--config", "/app/config.example.yaml"]
