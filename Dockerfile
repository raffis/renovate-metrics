FROM golang:1.22 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd cmd
COPY pkg pkg

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o renovate-metrics cmd/*

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/renovate-metrics /usr/bin/
USER 65532:65532

ENTRYPOINT ["/usr/bin/renovate-metrics"]
