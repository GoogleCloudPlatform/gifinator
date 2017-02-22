all:
	go build -o frontend/frontend ./frontend
	go build -o gifcreator/gifcreator ./gifcreator
	go build -o movie/movie ./movie
	go build -o render/render ./render

proto: proto/gifcreator.pb.go proto/movie.pb.go proto/render.pb.go

proto/gifcreator.pb.go proto/movie.pb.go proto/render.pb.go: proto/gifcreator.proto proto/movie.proto proto/render.proto
	protoc $^ --go_out=plugins=grpc:.

clean:
	rm -f gifcreator/gifcreator movie/movie render/render

.PHONY: all proto clean
