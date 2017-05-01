# Kubernetes Render Demo

This is a demonstration of how to build an application using Go, gRPC and Kubernetes. The application - Gifinator -
creates 3D animated gifs for no obvious purpose (except to show how these technologies can be used together).

This project is a compliment to a talk given at GCP Next 2017, for a demo and a walk-through of the application and the design choices we made, you can [watch the session on YouTube](https://www.youtube.com/watch?v=YiNt4kUnnIM).

This project currently assumes an import path of `github.com/GoogleCloudPlatform/gifinator`.

## Building

### Prerequisites

This project relies on [Glide](https://glide.sh) for dependency management and optionally [Goreman](https://github.com/mattn/goreman) (a clone of the popular Foreman tool, but written in Go) to
make it easy to run the services locally for testing purposes.

```bash
git clone ... $GOPATH/src/github.com/GoogleCloudPlatform/gifinator
curl https://glide.sh/get | sh
go get github.com/mattn/goreman
```

You will also need to install the [Google Cloud SDK](https://cloud.google.com/sdk/downloads) to build
and deploy to Kubernetes.

You will also need to have created a Google Cloud Storage bucket, for exclusive
use by the application. If you plan to deploy to GKE, it is suggested to
create your bucket in the same project.

To run and test locally, you will also need to have Redis installed and running.

### Building Locally

```bash
cd $GOPATH/src/github.com/GoogleCloudPlatform/gifinator
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
