/*
 * Copyright 2017 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
	pb "github.com/GoogleCloudPlatform/gifinator/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// TODO(jessup) remove globals in favor of appContext
var (
	templatePath string
	staticPath   string
	projectID    string // Google Console Project ID
	port         string

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
		span := traceClient.NewSpan("/memecreate") // TODO(jbd): make /memcreate top-level span optional
		defer span.Finish()
		response, err :=
			gcClient.StartJob(trace.NewContext(context.Background(), span),
				&pb.StartJobRequest{Name: gifName, ProductToPlug: mascotType})
		if err != nil {
			// TODO(jessup) Swap these out for proper logging
			fmt.Fprintf(os.Stderr, "cannot request Gif - %v", err)
			return
		}
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
	pathSegments := strings.Split(r.URL.Path, "/")
	if len(pathSegments) < 2 {
		http.Error(w, "Can't find the GIF ID", 404)
		return
	}

	// TODO(jessup) Look up to see if the gif has loaded. If not, show the Spinner.
	response, err :=
		gcClient.GetJob(
			context.Background(),
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
	pathSegments := strings.Split(r.URL.Path, "/")
	if len(pathSegments) < 2 {
		http.Error(w, "Can't find the GIF ID", 404)
		return
	}

	// TODO(jessup) Need stronger input validation here.
	response, err :=
		gcClient.GetJob(
			context.Background(),
			&pb.GetJobRequest{JobId: pathSegments[2]})
	if err != nil {
		// TODO(jessup) Swap these out for proper logging
		fmt.Fprintf(os.Stderr, "cannot get status of gif - %v", err)
		return
	}

	jsonReponse, _ := json.Marshal(response)
	fmt.Fprintf(w, string(jsonReponse))
}
