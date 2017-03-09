package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"math/rand"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/k8s-render-demo/internal/gcsref"
	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"github.com/fogleman/pt/pt"
)

type server struct{}

var (
	gcsClient *storage.Client
	gcsCacheDir string
)

func cacheGcsObject(ctx context.Context, obj gcsref.Object) (string, error) {
	// TODO(jessup) This will have collisions! Fix.
	localFilepath := gcsCacheDir+"/"+obj.Name

  // TODO(jessup) Check if file exists before pulling from disk
	fmt.Fprintf(os.Stdout, "DEBUG %s: %s", string(obj.Bucket), obj.Name)

	rc, err := gcsClient.Bucket(string(obj.Bucket)).Object(obj.Name).NewReader(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	// TODO(jessup) Fix this to read incrementally and not store files in memory
	slurp, err := ioutil.ReadAll(rc)
  if err != nil {
    fmt.Fprintf(os.Stderr, "readFile: unable to read data from bucket %q, file %q: %v", obj.Bucket, localFilepath, err)
    return "", err
  }
  fmt.Fprintf(os.Stdout, "writing file %q", localFilepath)
  err = ioutil.WriteFile(localFilepath, slurp, 0777)
  if err != nil {
		fmt.Fprintf(os.Stderr, "error while writing file %q: %v", localFilepath, err)
		return "", err
	}

	return localFilepath, nil
}

func renderImage(objectPath string, rotation float64, iterations int32) (string, error) {
	scene := pt.Scene{}

	// create materials
  objMat := pt.GlossyMaterial(pt.Black, 1.2, pt.Radians(30))
	wall := pt.GlossyMaterial(pt.HexColor(0xFCFAE1), 1.5, pt.Radians(10))
	light := pt.LightMaterial(pt.White, 80)

	// add walls and lights
	scene.Add(pt.NewCube(pt.V(-10, -1, -10), pt.V(-2, 10, 10), wall))
	scene.Add(pt.NewCube(pt.V(-10, -1, -10), pt.V(10, 0, 10), wall))
	scene.Add(pt.NewSphere(pt.V(4, 10, 1), 1, light))

	// load and transform gopher mesh
	mesh, err := pt.LoadOBJ(objectPath, objMat)
	if err != nil {
		return "", err
	}

	mesh.Transform(pt.Rotate(pt.V(0, 1, 0), pt.Radians(rotation)))
	mesh.SmoothNormals()
	mesh.FitInside(pt.Box{pt.V(-1, 0, -1), pt.V(1, 2, 1)}, pt.V(0.5, 0, 0.5))
	scene.Add(mesh)

	// position camera
	camera := pt.LookAt(pt.V(4, 1, 0), pt.V(0, 0.9, 0), pt.V(0, 1, 0), rotation)

	// render the scene
	sampler := pt.NewSampler(16, 16)
	renderer := pt.NewRenderer(&scene, &camera, sampler, 300, 300)

  // TODO(jessup) Fix this for better entropy
	imagePath := os.TempDir() + "/final_img_itr_%d_" + strconv.FormatInt(int64(rand.Intn(10000)), 16) + ".png"
	renderer.IterativeRender(imagePath, int(iterations))

	return fmt.Sprintf(imagePath, iterations), nil
}


func (server) RenderFrame(ctx context.Context, req *pb.RenderRequest) (*pb.RenderResponse, error) {
	gcsClient, _ = storage.NewClient(ctx)

	fmt.Fprintf(os.Stdout, "starting render job - object: %s, angle: %f\n", req.ObjPath, req.Rotation)

  // Load main object file
	objGcsObj, _ := gcsref.Parse(req.ObjPath)
  objFilepath, err := cacheGcsObject(ctx, objGcsObj)
	if err != nil {
		return nil, err
	}

	// Load the assets
  for _,element := range req.Assets {
		assetGcsObj, _ := gcsref.Parse(element)
	  _, err := cacheGcsObject(ctx, assetGcsObj)
		if err != nil {
			return nil, err
		}
	}

  // Create and render a scene seeded with the object we loaded
  imgPath, err := renderImage(objFilepath, float64(req.Rotation), req.Iterations)

	// Save in GCS
	gcsPath := fmt.Sprintf("%s.image_%.0frad.png", req.GcsOutputBase, req.Rotation)
	gcsFinalImageObj, err := gcsref.Parse(gcsPath)

	wc := gcsClient.Bucket(string(gcsFinalImageObj.Bucket)).Object(gcsFinalImageObj.Name).NewWriter(ctx)
  defer wc.Close()

	wc.ObjectAttrs.ContentType = "image/png"
	fmt.Fprintf(os.Stdout, "starting writing frame: %s from %s, frame: %f\n", gcsPath, imgPath, req.Rotation)

	// TODO(jessup) Do this iteratively to save memory
  contents, err := ioutil.ReadFile(imgPath)
	if err != nil {
		return nil, err
	}

	if _, err := wc.Write(contents); err != nil {
		return nil, err
	}

	response := pb.RenderResponse{GcsOutput: gcsPath}
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
	gcsCacheDir	= os.TempDir()

	srv := grpc.NewServer()
	pb.RegisterRenderServer(srv, server{})
	srv.Serve(l)
}
