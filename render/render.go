package main

import (
	"fmt"
	"image/gif"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"cloud.google.com/go/trace"
	"github.com/GoogleCloudPlatform/k8s-render-demo/internal/gcsref"
	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"github.com/anthonynsimon/bild/transform"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type server struct{}

const serviceName = "render"

var (
	gcsClient 	 *storage.Client
	deploymentId string
	traceClient	 *trace.Client
)

func (server) RenderFrame(ctx context.Context, req *pb.RenderRequest) (*pb.RenderResponse, error) {
	md, _ := metadata.FromContext(ctx)
  parentSpan := traceClient.SpanFromHeader(
    "frontend.handleForm", strings.Join(md["trace_header"], ""),
  )
	span := parentSpan.NewChild("render.RenderFrame")
	span.SetLabel("service", serviceName)
	span.SetLabel("version", deploymentId)
	defer span.Finish()


  tCtx := trace.NewContext(ctx, span)

	gcsClient, err := storage.NewClient(tCtx)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stdout, "starting render job - object: %s, frame: %d\n", req.ImgPath, req.Frame)

	gcsImageImportObj, err := gcsref.Parse(req.ImgPath)
	rc, err := gcsClient.Bucket(string(gcsImageImportObj.Bucket)).Object(gcsImageImportObj.Name).NewReader(tCtx)
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
	wc := gcsClient.Bucket(string(gcsFinalImageObj.Bucket)).Object(gcsFinalImageObj.Name).NewWriter(tCtx)
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
	projectId := os.Getenv("GOOGLE_PROJECT_ID")
	deploymentId = os.Getenv("DEPLOYMENT_ID")

	ctx := context.Background()
	tc, err := trace.NewClient(ctx, projectId, trace.EnableGRPCTracing)
	if err != nil {
		log.Fatal(err)
	}
	traceClient = tc

	l, err := net.Listen("tcp", ":"+serving_port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
		return
	}
	srv := grpc.NewServer()
	pb.RegisterRenderServer(srv, server{})
	srv.Serve(l)
}
