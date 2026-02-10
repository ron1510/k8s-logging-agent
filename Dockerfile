FROM golang:1.22 as builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent ./cmd/agent

FROM alpine:3.20
RUN adduser -D -g '' agent && mkdir -p /var/log/agent && chown -R agent:agent /var/log/agent
USER agent
WORKDIR /app
COPY --from=builder /out/agent /app/agent
ENV LOG_LEVEL=info
CMD ["/bin/sh", "-c", "/app/agent | tee /var/log/agent/agent.log"]
