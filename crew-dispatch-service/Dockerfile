FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/crew-dispatch-service ./crew-dispatch-service/cmd/crew-dispatch-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/crew-dispatch-service /crew-dispatch-service
EXPOSE 8085
ENTRYPOINT ["/crew-dispatch-service"]
