FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/gate-service ./gate-service/cmd/gate-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/gate-service /gate-service
EXPOSE 8081
ENTRYPOINT ["/gate-service"]
