FROM golang:latest AS golang
ENV GOPATH /go
WORKDIR /go/src/github.com/cisco-sso/snapshot-validator
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o snapshot-validator .

FROM scratch
COPY --from=golang /go/src/github.com/cisco-sso/snapshot-validator/snapshot-validator /
ENTRYPOINT ["/snapshot-validator"]
