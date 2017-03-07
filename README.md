# Kubernetes Render Demo

Currently assuming an import path of `github.com/GoogleCloudPlatform/k8s-render-demo`.

## Building

### Prerequisites

```bash
git clone ... $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
curl https://glide.sh/get | sh
go get github.com/mattn/goreman
```

You will also need to install the [Google Cloud SDK](https://cloud.google.com/sdk/downloads) to build
and deploy to Kubernetes.

You will also need to have created a Google Cloud Storage bucket, for exclusive
use by the application.

To run and test locally, you will also need to have Redis installed and running.

### Building Locally

```bash
cd $GOPATH/src/github.com/GoogleCloudPlatform/k8s-render-demo
glide install
make
```

### Regenerating Protos

If you need to rebuild the generated code for the protos, then install `protoc`
and run `make proto`.

## Running Locally

Configure `.env` as appropriate. By default it assumes everything is running on
localhost, including Redis.

```bash
gcloud auth application-default login # first time only
export $(cat .env | xargs)
make && goreman start
```

Ignore the port numbers, as they are specified in the `.env` file.

If you run into the gopkg.in issue, then run:

```bash
git clone https://p3.gopkg.in/yaml.v2 $GOPATH/src/gopkg.in/yaml.v2
```

## Building the container image with Container Builder

A single image contains all three binaries, along with assets for the web-server
frontend.

```bash
gcloud container builds submit . --config=cloudbuild.yaml
```

This will build and push the image to `gcr.io/YOUR_PROJECT_ID/gifcreator`

## Deploy the solution to a Kubernetes cluster

First create a Kubernetes cluster and make sure `kubectl` is installed and configured
to talk to it.

Configure the files in the `k8s` directory as appropriate. Mainly this will mean
adjusting the value of the `GOOGLE_PROJECT_ID` and `GCS_BUCKET_NAME` to something
appropriate for your usage.

To deploy the three services, and Redis, to the cluster for the first time, run:
```bash
kubectl create -f k8s
```
