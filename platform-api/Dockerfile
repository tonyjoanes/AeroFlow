FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/platform-api ./platform-api/cmd/platform-api

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/platform-api /platform-api
EXPOSE 9000
ENTRYPOINT ["/platform-api"]
