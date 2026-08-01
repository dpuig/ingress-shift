# Builds any one of the three Ingress Shift tools. Defaults to the
# open-source analyzer; override TOOL to build the harness or orchestrator:
#   docker build --build-arg TOOL=harness -t ingress-shift-harness .
#   docker build --build-arg TOOL=orchestrator -t ingress-shift-orchestrator .
FROM golang:1.21-alpine AS builder

ARG TOOL=analyzer

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/ingress-shift ./src/${TOOL}

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ingress-shift .
CMD ["./ingress-shift"]
