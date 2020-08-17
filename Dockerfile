FROM golang:1.14 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -v -o /bin/cloud_run_release_manager ./cmd/operator

FROM gcr.io/distroless/static
COPY --from=builder /bin/cloud_run_release_manager /bin/cloud_run_release_manager
ENTRYPOINT [ "/bin/cloud_run_release_manager" ]
