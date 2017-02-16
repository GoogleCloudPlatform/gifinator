# Kubernetes Render Demo

Currently assuming an import path of `github.com/GoogleCloudPlatform/k8s-render-demo`.

## Building

### Prerequisites

```bash
git clone ... $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
go get -u github.com/golang/dep/cmd/dep
```

### Building

```bash
cd $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
dep ensure
make
```

### Regenerating Protos

If you need to rebuild the generated code for the protos, then install `protoc` and run `make proto`.

## Running

```bash
go get github.com/mattn/goreman
make && goreman start
```

Ignore the port numbers, as they are specified in the `.env` file.

If you run into the gopkg.in issue, then run:

```bash
git clone https://p3.gopkg.in/yaml.v2 $GOPATH/src/gopkg.in/yaml.v2
```
