FROM golang:1.23-alpine AS builder

RUN apk add --no-cache build-base linux-headers

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o state-patcher .

FROM alpine:3.20
COPY --from=builder /build/state-patcher /usr/local/bin/
ENTRYPOINT ["state-patcher"]
