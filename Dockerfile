FROM golang:1.22 as builder
ENV GOOS=linux
ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /src

# Copy the Go Modules manifests
COPY go.* .

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Build dependencies
RUN go build std

# Copy rest of project
COPY . /src

# Run tests
RUN make test

# Build
RUN make build


FROM gcr.io/distroless/static-debian11
WORKDIR /app
COPY --from=builder /src/bin/netroll /app/netroll
ENTRYPOINT ["/app/netroll"]