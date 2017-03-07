package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"

	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"github.com/anthonynsimon/bild/transform"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct{}

func (server) RenderFrame(ctx context.Context, req *pb.RenderRequest) (*pb.RenderResponse, error) {
	// TODO: read file from GCS
	wdPath, _ := os.Getwd()
	r, err := os.Open(wdPath+req.ImgPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	deg := float64(req.Frame * 10)
	rotated := transform.Rotate(img, deg, nil)

	// TODO: store in GCS
	tempPath := req.GcsOutputBase+fmt.Sprintf("%s%.0f", "/image_", deg)
	file, err := os.Create(tempPath)
	if err != nil {
		// TODO: response?
		return nil, err
	}
	defer file.Close()

	err = png.Encode(file, rotated)
	if err != nil {
		// TODO: response?
		return nil, err
	}

	response := pb.RenderResponse{GcsOutput: tempPath}
	return &response, nil
}

func main() {
	serving_port := os.Getenv("RENDER_PORT")
	i, err := strconv.Atoi(serving_port)
	if (err != nil) || (i < 1) {
		log.Fatalf("please set env var RENDER_PORT to a valid port")
	}

	l, err := net.Listen("tcp", ":"+serving_port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterRenderServer(srv, server{})
	srv.Serve(l)
}
