FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/notification-service ./notification-service/cmd/notification-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/notification-service /notification-service
EXPOSE 8086
ENTRYPOINT ["/notification-service"]
