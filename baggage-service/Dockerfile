FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/baggage-service ./baggage-service/cmd/baggage-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/baggage-service /baggage-service
EXPOSE 8082
ENTRYPOINT ["/baggage-service"]
