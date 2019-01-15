FROM golang:latest AS golang
ENV GOPATH /go
WORKDIR /go/src/github.com/cisco-sso/snapshot-manager
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o snapshot-manager .

FROM ubuntu
COPY --from=golang /go/src/github.com/cisco-sso/snapshot-manager/snapshot-manager /usr/local/bin/
ENTRYPOINT ["snapshot-manager"]
