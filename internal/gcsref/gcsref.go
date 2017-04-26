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

// Package gcsref provides types for referencing Google Cloud Storage
// buckets and objects.
package gcsref

import (
	"fmt"
	"net/url"
	"strings"
)

// MustParse parses a "gs://" URI into an object reference or panics.
func MustParse(uri string) Object {
	o, err := Parse(uri)
	if err != nil {
		panic(err)
	}
	return o
}

// Parse parses a "gs://" URI into an object reference.
func Parse(uri string) (Object, error) {
	const prefix = "gs://"
	if !strings.HasPrefix(uri, prefix) {
		return Object{}, fmt.Errorf("parse GCS URI %q: scheme is not %q", uri, prefix)
	}
	uri = uri[len(prefix):]
	i := strings.IndexByte(uri, '/')
	if i == -1 {
		return Object{}, fmt.Errorf("parse GCS URI %q: no object name", uri)
	}
	bucket, name := Bucket(uri[:i]), uri[i+1:]
	if !bucket.IsValid() {
		return Object{}, fmt.Errorf("parse GCS URI %q: invalid bucket %q", uri, string(bucket))
	}
	obj := bucket.Object(name)
	if !obj.IsValid() {
		return Object{}, fmt.Errorf("parse GCS URI %q: invalid object %q", uri, name)
	}
	return obj, nil
}

// Bucket is a reference to a Cloud Storage bucket.
type Bucket string

// String returns the bucket name.
func (b Bucket) String() string {
	return string(b)
}

// Object returns a reference for an object inside the bucket.
func (b Bucket) Object(name string) Object {
	return Object{Bucket: b, Name: name}
}

// IsValid reports whether the bucket name is valid.
// Note that this check is optimized to be fast, not absolutely correct, so some
// bucket names that are reported valid by this function may be considered
// invalid by the API.
func (b Bucket) IsValid() bool {
	const (
		bucketNormalMax = 63
		bucketDotsMax   = 222
	)
	n, hasDots := 0, false
	for _, c := range b {
		n++
		if n > bucketDotsMax {
			return false
		}
		if c == '.' {
			hasDots = true
			continue
		}
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return n >= 3 && (hasDots && n <= bucketDotsMax || !hasDots && n <= bucketNormalMax)
}

// Object is a reference to a Cloud Storage object.
type Object struct {
	Name   string
	Bucket Bucket
}

// String returns the object as a "gs://" URI.
func (o Object) String() string {
	return "gs://" + string(o.Bucket) + "/" + o.Name
}

// DownloadURL returns the URL for downloading the object over HTTP.
func (o Object) DownloadURL() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", escapePath(string(o.Bucket)), escapePath(o.Name)),
	}
}

// IsValid reports whether the object name is valid.
func (o Object) IsValid() bool {
	const maxSize = 1024
	if o.Name == "" || !o.Bucket.IsValid() {
		return false
	}
	n := 0
	for _, c := range o.Name {
		n++
		if n > maxSize || c == '\r' || c == '\n' || c == 0xFFFD {
			return false
		}
	}
	return true
}

func escapePath(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return s
	}

	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case shouldEscape(c):
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

// Return true if the specified character should be escaped when
// appearing in a URL string, according to RFC 3986.
//
// Please be informed that for now shouldEscape does not check all
// reserved characters correctly. See golang.org/issue/5684.
func shouldEscape(c byte) bool {
	// §2.3 Unreserved characters (alphanum)
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '-', '_', '.', '~': // §2.3 Unreserved characters (mark)
		return false

	case '$', '&', '+', ',', '/', ':', ';', '=', '?', '@': // §2.2 Reserved characters (reserved)
		// The RFC allows : @ & = + $ but saves / ; , for assigning
		// meaning to individual path segments. This package
		// only manipulates the path as a whole, so we allow those
		// last two as well. That leaves only ? to escape.
		return c == '?'
	}

	// Everything else must be escaped.
	return true
}
