FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /gomailtest ./cmd/gomailtest


FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /gomailtest /usr/local/bin/gomailtest

EXPOSE 8080

ENTRYPOINT ["gomailtest"]
CMD ["serve"]
