# -- Build stage --
FROM golang:1.26-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o api ./cmd/api

# -- Final stage --
FROM alpine:3.21

WORKDIR /app

COPY --from=build /app/api .

EXPOSE 8080

CMD ["./api"]
