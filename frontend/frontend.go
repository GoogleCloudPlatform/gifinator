package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var template_path, static_path string
var serving_port string

func main() {
	template_path = os.Getenv("FRONTEND_TEMPLATES_DIR")
	static_path = os.Getenv("FRONTEND_STATIC_DIR")
	serving_port = os.Getenv("FRONTEND_PORT")

	fs := http.FileServer(http.Dir(static_path))

	http.HandleFunc("/", handleForm)
	http.HandleFunc("/gif/", handleGif)
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.ListenAndServe(":"+serving_port, nil)
}

func handleForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// Get the form info, verify, and pass on
		var formErrors = []string{}
		var gifName, mascotType string
		r.ParseForm()
		if (r.Form["name"] != nil) && (len(r.Form["name"][0]) > 0) {
			gifName = r.Form["name"][0]
		} else {
			formErrors = append(formErrors, "Please provide a name")
		}
		if r.Form["mascot"] != nil {
			mascotType = r.Form["mascot"][0]
		} else {
			formErrors = append(formErrors, "Please specify a mascot")
		}
		if len(formErrors) > 0 {
			renderForm(w, formErrors)
			return
		} else {
			// Submit answers, get task ID, and redirect...
			errString := fmt.Sprintf("TODO (jessup) Submit %s with %s", gifName, mascotType)
			http.Error(w, errString, 404)
		}
	} else {
		renderForm(w, nil)
		return
	}
}

func renderForm(w http.ResponseWriter, errors []string) {
	// Show the form
	formPath := filepath.Join(template_path, "form.html")
	layoutPath := filepath.Join(template_path, "layout.html")

	t, err := template.ParseFiles(layoutPath, formPath)
	if err == nil {
		t.ExecuteTemplate(w, "layout", errors)
	} else {
		http.Error(w, err.Error(), 500)
	}
}

func handleGif(w http.ResponseWriter, r *http.Request) {
	pathSegments := strings.Split(r.URL.Path, "/")
	if len(pathSegments) < 2 {
		http.Error(w, "Can't find the GIF ID", 404)
		return
	}

	// TODO(jessup) Look up to see if the gif has loaded. If not, show the Spinner.

	formPath := filepath.Join(template_path, "spinner.html")
	layoutPath := filepath.Join(template_path, "layout.html")

	t, err := template.ParseFiles(layoutPath, formPath)
	if err == nil {
		t.ExecuteTemplate(w, "layout", pathSegments[2])
	} else {
		http.Error(w, err.Error(), 500)
	}
}
