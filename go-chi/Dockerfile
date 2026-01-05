FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOEXPERIMENT=jsonv2

RUN go build -trimpath -ldflags="-s -w" -o server ./cmd/main.go

FROM alpine

WORKDIR /app

ENV PORT=8080 \
    HOST=0.0.0.0

COPY --from=builder /app/server .

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 5000

CMD ["./server"]