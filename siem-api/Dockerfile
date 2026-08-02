FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/siem-api ./cmd/siem-api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/siem-api /usr/local/bin/siem-api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/siem-api"]
