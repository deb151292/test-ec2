From alpine:latest as builder

Run apk add --no-cache build-base go

WORKDIR /usr/src/app

copy go.sum go.mod ./

Run go mod download

copy . .

Run go build -o app .

From alpine:latest

WORKDIR /app

Copy --from=builder /usr/src/app/app .

Expose 8000

CMD ["./app"]







