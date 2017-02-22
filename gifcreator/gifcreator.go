package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"math/rand"

	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct{}

func (server) StartJob(ctx context.Context, req *pb.StartJobRequest) (*pb.StartJobResponse, error) {
	// TODO(jessup) this should be stored as a job in Redis
	response := pb.StartJobResponse{ JobId: strconv.Itoa(rand.Intn(1000000)) }
	return &response, nil
}

func (server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	// TODO(jessup) look this up from a Reids service
	var status pb.GetJobResponse_Status
	switch rand.Intn(2) {
	case 0:
		status = pb.GetJobResponse_UNKNOWN_STATUS
	case 1:
		status = pb.GetJobResponse_PENDING
	case 2:
		status = pb.GetJobResponse_DONE
	}
	response := pb.GetJobResponse{ ImageUrl: "", Status: status }
	return &response, nil
}

func main() {
	serving_port := os.Getenv("GIFCREATOR_PORT")
	i, err := strconv.Atoi(serving_port)
	if (err != nil) || (i < 1) {
		log.Fatalf("please set env var GIFCREATOR_PORT to a valid port")
	}

	l, err := net.Listen("tcp", ":"+serving_port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterGifCreatorServer(srv, server{})
	srv.Serve(l)
}
