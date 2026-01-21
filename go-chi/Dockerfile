FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux

RUN go build -trimpath -ldflags="-s -w" -o server ./cmd/main.go

FROM alpine

WORKDIR /app

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=5001

COPY --from=builder /app/server .

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 5001

CMD ["./server"]
