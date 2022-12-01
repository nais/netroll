FROM golang:1.19-alpine as builder
RUN apk add --no-cache git make
ENV GOOS=linux
ENV CGO_ENABLED=0
ENV GO111MODULE=on
COPY . /src
WORKDIR /src
RUN rm -f go.sum
RUN go get ./...
RUN go build -a -installsuffix cgo -o bin/netroll cmd/netroll/main.go

FROM gcr.io/distroless/base
WORKDIR /app
COPY --from=builder /src/bin/netroll /app/netroll
ENTRYPOINT ["/app/netroll"]