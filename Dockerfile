FROM golang:latest AS golang
ENV GOPATH /go
WORKDIR /go/src/github.com/cisco-sso/snapshot-manager
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o snapshot-manager .

FROM ubuntu
RUN apt-get update -y && apt-get install curl -y
RUN curl -sSLo /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.12.2/bin/linux/amd64/kubectl && chmod +x /usr/local/bin/kubectl 
COPY --from=golang /go/src/github.com/cisco-sso/snapshot-manager/snapshot-manager /usr/local/bin/
ENTRYPOINT ["snapshot-manager"]
