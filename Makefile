all:
	go build -o gifcreator/gifcreator ./gifcreator
	go build -o movie/movie ./movie
	go build -o render/render ./render

proto: proto/gifcreator.pb.go proto/movie.pb.go proto/render.pb.go

proto/gifcreator.pb.go proto/movie.pb.go proto/render.pb.go: proto/gifcreator.proto proto/movie.proto proto/render.proto
	protoc $^ --go_out=plugins=grpc:.

clean:
	rm -f gifcreator/gifcreator movie/movie render/render

run:
	export FRONTEND_PORT=8080; \
	export FRONTEND_TEMPLATES_DIR=frontend/templates; \
	export FRONTEND_STATIC_DIR=frontend/static; \
	go run frontend/*

.PHONY: all proto clean
