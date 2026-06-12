FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/carousel-service ./carousel-service/cmd/carousel-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/carousel-service /carousel-service
EXPOSE 8083
ENTRYPOINT ["/carousel-service"]
