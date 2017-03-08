package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/trace"
	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const serviceName = "frontend"

// TODO(jessup) remove globals in favor of appContext
var (
	templatePath string
	staticPath   string
	projectID    string // Google Console Project ID
	port         string
	deploymentId string

	gcClient    pb.GifCreatorClient
	traceClient *trace.Client
)

func main() {
	// TODO(jbd): convert env vars into flags
	templatePath = os.Getenv("FRONTEND_TEMPLATES_DIR")
	staticPath = os.Getenv("FRONTEND_STATIC_DIR")
	projectID = os.Getenv("GOOGLE_PROJECT_ID")
	port = os.Getenv("FRONTEND_PORT")
	gifcreatorPort := os.Getenv("GIFCREATOR_PORT")
	gifcreatorName := os.Getenv("GIFCREATOR_NAME")
  deploymentId = os.Getenv("DEPLOYMENT_ID")
	// TODO(jessup): check env vars for correctnesss

	fs := http.FileServer(http.Dir(staticPath))
	gcHostAddr := gifcreatorName + ":" + gifcreatorPort

	ctx := context.Background()
	tc, err := trace.NewClient(ctx, projectID, trace.EnableGRPCTracing)
	if err != nil {
		log.Fatal(err)
	}
	traceClient = tc

	// TODO(jessup) Create TLS certs
	conn, err := grpc.Dial(gcHostAddr,
		trace.EnableGRPCTracingDialOption, grpc.WithInsecure())
	if err != nil {
		// TODO(jessup) Swap these out for proper logging
		fmt.Fprintf(os.Stderr, "cannot connect to gifcreator %s\n%v", gcHostAddr, err)
		return
	}
	defer conn.Close()

	gcClient = pb.NewGifCreatorClient(conn)

	http.HandleFunc("/", handleForm)
	http.HandleFunc("/gif/", handleGif)
	http.HandleFunc("/check/", handleGifStatus)
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.ListenAndServe(":"+port, nil)
}

func handleForm(w http.ResponseWriter, r *http.Request) {
	parentSpan := traceClient.NewSpan("create_gif") // TODO(jbd): make /memcreate top-level span optional
	span := parentSpan.NewChild("frontend.handleForm")
	fmt.Fprintf(os.Stderr, "fe trace contexts: parent %s child %s", parentSpan.TraceID(), span.TraceID())
	span.SetLabel("version", deploymentId)
	span.SetLabel("service", serviceName)
	defer span.Finish()
	tCtx := trace.NewContext(context.Background(),span)

	if r.Method == "POST" {
		// Get the form info, verify, and pass on
		var formErrors = []string{}
		var gifName string
		var mascotType pb.Product
		r.ParseForm()
		if (r.Form["name"] != nil) && (len(r.Form["name"][0]) > 0) {
			gifName = r.Form["name"][0]
		} else {
			formErrors = append(formErrors, "Please provide a name")
		}
		if r.Form["mascot"] != nil {
			switch r.Form["mascot"][0] {
			case "go":
				mascotType = pb.Product_GO
			case "grpc":
				mascotType = pb.Product_GRPC
			case "kubernetes":
				mascotType = pb.Product_KUBERNETES
			default:
				mascotType = pb.Product_UNKNOWN_PRODUCT
			}
		} else {
			formErrors = append(formErrors, "Please specify a mascot")
		}
		if len(formErrors) > 0 {
			renderForm(w, formErrors)
			return
		}
		// Submit answers, get task ID, and redirect...
		span.SetLabel("mascot", string(pb.Product_GO))
		response, err :=
			gcClient.StartJob(tCtx, &pb.StartJobRequest{Name: gifName, ProductToPlug: mascotType})
		if err != nil {
			// TODO(jessup) Swap these out for proper logging
			fmt.Fprintf(os.Stderr, "cannot request Gif - %v", err)
			return
		}
		parentSpan.SetLabel("job_id", response.JobId)
		http.Redirect(w, r, "/gif/"+response.JobId, 301)
		return
	}
	renderForm(w, nil)
	return
}

func renderForm(w http.ResponseWriter, errors []string) {
	// Show the form
	formPath := filepath.Join(templatePath, "form.html")
	layoutPath := filepath.Join(templatePath, "layout.html")

	t, err := template.ParseFiles(layoutPath, formPath)
	if err == nil {
		t.ExecuteTemplate(w, "layout", errors)
	} else {
		http.Error(w, err.Error(), 500)
	}
}

type responsePageData struct {
	ImageId  string
	ImageUrl string
}

func handleGif(w http.ResponseWriter, r *http.Request) {
	span := traceClient.NewSpan("frontend.handleGif") // TODO(jbd): make /memcreate top-level span optional
	span.SetLabel("service", serviceName)
	span.SetLabel("version", deploymentId)
	defer span.Finish()

	pathSegments := strings.Split(r.URL.Path, "/")
	if len(pathSegments) < 2 {
		http.Error(w, "Can't find the GIF ID", 404)
		return
	}

	response, err :=
		gcClient.GetJob(
			trace.NewContext(context.Background(), span),
			&pb.GetJobRequest{JobId: pathSegments[2]})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get status of gif - %v", err)
		return
	}

	var bodyHtmlPath string
	var gifInfo = responsePageData{
		ImageId: pathSegments[2],
	}
	switch response.Status {
	case pb.GetJobResponse_PENDING:
		bodyHtmlPath = filepath.Join(templatePath, "spinner.html")
		break
	case pb.GetJobResponse_DONE:
		bodyHtmlPath = filepath.Join(templatePath, "gif.html")
		gifInfo.ImageUrl = response.ImageUrl
		break
	default:
		bodyHtmlPath = filepath.Join(templatePath, "error.html")
		break
	}
	layoutPath := filepath.Join(templatePath, "layout.html")

	t, err := template.ParseFiles(layoutPath, bodyHtmlPath)
	if err == nil {
		t.ExecuteTemplate(w, "layout", gifInfo)
	} else {
		http.Error(w, err.Error(), 500)
	}
}

func handleGifStatus(w http.ResponseWriter, r *http.Request) {
	span := traceClient.NewSpan("frontend.handleGifStatus") // TODO(jbd): make /memcreate top-level span optional
	span.SetLabel("service", serviceName)
	span.SetLabel("version", deploymentId)
	defer span.Finish()

	pathSegments := strings.Split(r.URL.Path, "/")
	if len(pathSegments) < 2 {
		http.Error(w, "Can't find the GIF ID", 404)
		return
	}

	// TODO(jessup) Need stronger input validation here.
	response, err :=
		gcClient.GetJob(
			trace.NewContext(context.Background(), span),
			&pb.GetJobRequest{JobId: pathSegments[2]})
	if err != nil {
		// TODO(jessup) Swap these out for proper logging
		fmt.Fprintf(os.Stderr, "cannot get status of gif - %v", err)
		return
	}

	jsonReponse, _ := json.Marshal(response)
	fmt.Fprintf(w, string(jsonReponse))
}
