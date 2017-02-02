# Kubernetes Render Demo

Currently assuming an import path of `github.com/GoogleCloudPlatform/k8s-render-demo`.

## Building

### Prerequisites

```bash
git clone ... $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
go get -u github.com/golang/dep
```

### Building

```bash
cd $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
dep ensure
go build ./...
```

### Regenerating Protos

If you need to rebuild the generated code for the protos, then install `protoc` and run `make proto`.
