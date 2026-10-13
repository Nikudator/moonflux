FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/moonflux ./cmd/moonflux

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S moonflux \
    && adduser -S -G moonflux moonflux

WORKDIR /app

COPY --from=builder /out/moonflux /app/moonflux
COPY config/ /app/config/
COPY migrations/ /app/migrations/

USER moonflux

EXPOSE 8080

ENTRYPOINT ["/app/moonflux"]