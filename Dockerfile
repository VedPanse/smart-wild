FROM golang:1.25.5-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o orchestrator .

FROM alpine:3.22

WORKDIR /app

COPY --from=build /app/orchestrator /app/orchestrator
COPY index.html /app/index.html

ENV PORT=10000
EXPOSE 10000

CMD ["/app/orchestrator"]
