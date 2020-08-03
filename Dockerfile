FROM golang:1.14 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -v -o /bin/cloud_run_release_operator ./cmd/operator

FROM gcr.io/distroless/static
COPY --from=builder /bin/cloud_run_release_operator /bin/cloud_run_release_operator
ENTRYPOINT [ "/bin/cloud_run_release_operator", "-cli"]
