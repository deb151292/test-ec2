FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /usr/src/app/app .

EXPOSE 8000

ENV GIN_MODE=release

CMD ["./app"]
