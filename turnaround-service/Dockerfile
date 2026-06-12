FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/turnaround-service ./turnaround-service/cmd/turnaround-service

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/turnaround-service /turnaround-service
EXPOSE 8084
ENTRYPOINT ["/turnaround-service"]
