FROM golang:alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/scheduler .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /out/scheduler /app/scheduler
COPY web /app/web

VOLUME ["/data"]

CMD ["/app/scheduler"]
