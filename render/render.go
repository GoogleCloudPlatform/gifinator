package main

import (
	"fmt"
	"image/gif"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/k8s-render-demo/internal/gcsref"
	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"github.com/anthonynsimon/bild/transform"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct{}

var (
	gcsClient *storage.Client
)

func (server) RenderFrame(ctx context.Context, req *pb.RenderRequest) (*pb.RenderResponse, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stdout, "starting render job - object: %s, frame: %d\n", req.ImgPath, req.Frame)

	gcsImageImportObj, err := gcsref.Parse(req.ImgPath)
	rc, err := gcsClient.Bucket(string(gcsImageImportObj.Bucket)).Object(gcsImageImportObj.Name).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	img, err := png.Decode(rc)
	if err != nil {
		return nil, err
	}

	deg := float64(req.Frame * 10)
	rotated := transform.Rotate(img, deg, nil)

	// TODO: store in GCS
	tempPath := os.TempDir() + "/" + fmt.Sprintf("%s%.0f", "/image_", deg)
	file, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Save in GCS
	gcsPath := fmt.Sprintf("%s.image_%.0f.gif", req.GcsOutputBase, deg)
	gcsFinalImageObj, err := gcsref.Parse(gcsPath)
	wc := gcsClient.Bucket(string(gcsFinalImageObj.Bucket)).Object(gcsFinalImageObj.Name).NewWriter(ctx)
	defer wc.Close()

	var opt gif.Options
	opt.NumColors = 256

	wc.ObjectAttrs.ContentType = "image/gif"

	fmt.Fprintf(os.Stdout, "starting writing frame: %s, frame: %d\n", gcsPath, req.Frame)
	err = gif.Encode(wc, rotated, &opt)
	if err != nil {
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
		return
	}

	l, err := net.Listen("tcp", ":"+serving_port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
		return
	}
	srv := grpc.NewServer()
	pb.RegisterRenderServer(srv, server{})
	srv.Serve(l)
}
