package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/k8s-render-demo/internal/gcsref"
	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct {
	storage *storage.Client
}

func (s *server) MakeMovie(ctx context.Context, req *pb.MakeMovieRequest) (*pb.MakeMovieResponse, error) {
	if len(req.Frames) == 0 {
		return nil, errors.New("invalid request: no frames")
	}
	out, err := gcsref.Parse(req.GcsOutput)
	if err != nil {
		return nil, fmt.Errorf("invalid request: output URI: %v", err)
	}
	g := &gif.GIF{
		Image:  make([]*image.Paletted, len(req.Frames)),
		Delay:  make([]int, len(req.Frames)),
		Config: image.Config{ColorModel: color.Palette(palette.Plan9[:])},
	}
	for i, fname := range req.Frames {
		frame, err := loadFrame(ctx, s.storage, fname)
		if err != nil {
			return nil, fmt.Errorf("frame %d at %v: %v", i, fname, err)
		}
		bd := frame.Bounds()
		if i == 0 {
			// Pull dimensions off first frame.
			g.Config.Width = bd.Dx()
			g.Config.Height = bd.Dy()
		}
		dstBd := image.Rect(0, 0, g.Config.Width, g.Config.Height)
		g.Image[i] = image.NewPaletted(dstBd, palette.Plan9)
		draw.FloydSteinberg.Draw(g.Image[i], dstBd, frame, bd.Min)
	}
	w := objHandle(s.storage, out).NewWriter(ctx)
	w.ObjectAttrs.ContentType = "image/gif"
	err = gif.EncodeAll(w, g)
	cerr := w.Close()
	if err != nil {
		return nil, err
	}
	if cerr != nil {
		return nil, err
	}
	return new(pb.MakeMovieResponse), nil
}

func objHandle(client *storage.Client, ref gcsref.Object) *storage.ObjectHandle {
	return client.Bucket(string(ref.Bucket)).Object(ref.Name)
}

func loadFrame(ctx context.Context, client *storage.Client, uri string) (image.Image, error) {
	fobj, err := gcsref.Parse(uri)
	if err != nil {
		return nil, err
	}
	r, err := objHandle(client, fobj).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	if r.ContentType() != "image/png" {
		r.Close()
		return nil, fmt.Errorf("content type is %q instead of \"image/png\"", r.ContentType())
	}
	frame, err := png.Decode(r)
	cerr := r.Close()
	if err != nil {
		return nil, err
	}
	if cerr != nil {
		log.Printf("close frame reader for %s: %v", uri, err)
	}
	return frame, nil
}

func main() {
	serving_port := os.Getenv("MOVIE_PORT")
	i, err := strconv.Atoi(serving_port)
	if (err != nil) || (i < 1) {
		log.Fatalf("please set env var MOVIE_PORT to a valid port")
	}
	gcs, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatalf("dial storage: %v", err)
	}
	l, err := net.Listen("tcp", ":"+serving_port)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterSequencerServer(srv, &server{storage: gcs})
	srv.Serve(l)
}
