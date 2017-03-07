package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"encoding/json"
	"image"
	"image/gif"

	"gopkg.in/redis.v5"

	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"cloud.google.com/go/trace"
	"cloud.google.com/go/storage"

	"google.golang.org/api/iterator"
)

type server struct{}

type renderJob struct{
	Status					pb.GetJobResponse_Status
	FinalImagePath	string
}

type renderTask struct{
	Frame	 					int64
	Caption					string
	ProductType			pb.Product
}

var (
	redisClient   *redis.Client
	renderClient  pb.RenderClient
	workerMode    = flag.Bool("worker", false, "run in worker mode rather than server")
	traceClient   *trace.Client
	gcsBucketName	string
)

func (server) StartJob(ctx context.Context, req *pb.StartJobRequest) (*pb.StartJobResponse, error) {
	// Retrieive the next job ID from Redis
	jobId, err := redisClient.Incr("gifjob_counter").Result()
	if err != nil {
		return nil, err
	}
	jobIdStr := strconv.FormatInt(jobId, 10)

	// Create a new RenderJob queue for that job
  var job = renderJob{
		Status:	pb.GetJobResponse_PENDING,
	}
	payload, _ := json.Marshal(job)
	err = redisClient.Set("job_gifjob_"+jobIdStr, payload, 0).Err()
	if err != nil {
		return nil, err
	}

	// Add tasks to the GifJob queue for each frame to render
	var taskId int64
	for i := 0; i < 3; i++ {
		// Set up render request for each frame
    var task = renderTask{
			Frame: 				int64(i),
			ProductType: 	req.ProductToPlug,
			Caption:			req.Name,
		}

		//Get new task id
		taskId, err = redisClient.Incr("counter_queued_gifjob_" + jobIdStr).Result()
		if err != nil {
			return nil, err
		}
		taskIdStr := strconv.FormatInt(taskId, 10)

		payload, err = json.Marshal(task)
		if err != nil {
			return nil, err
		}
		err = redisClient.Set("task_gifjob_"+jobIdStr+"_"+taskIdStr, payload, 0).Err()
		if err != nil {
			return nil, err
		}
		err = redisClient.LPush("gifjob_queued", jobIdStr+"_"+taskIdStr).Err()
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stdout, "enqueued gifjob_%s_%s %s\n", jobIdStr, taskIdStr, payload)
	}

	// Return job ID
	response := pb.StartJobResponse{JobId: jobIdStr}

	return &response, nil
}

func leaseNextTask() error {
	/**
	 * We want to make task leasing as robust as possible. We do this by
	 * shifting the task marker to a 'processing' queue that signals that we are
	 * trying to work on it. Once the task is done it's removed from the
	 * processing queue. If this process crashes during processing then a garbage
	 * collector could move the task back into the 'queueing' queue.
	 */

	jobString, err := redisClient.BRPopLPush("gifjob_queued", "gifjob_processing", 0).Result()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "leased gifjob_%s\n", jobString)

	// extract task ID and job ID
	strs := strings.Split(jobString, "_")
	jobIdStr := strs[0]
	taskIdStr := strs[1]
	//taskId, _ := strconv.ParseInt(taskIdStr, 10, 64)

	payload, err := redisClient.Get("task_gifjob_"+jobIdStr+"_"+taskIdStr).Result()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "leased gifjob_%s %s\n", jobString, payload)

	var task renderTask
  err = json.Unmarshal([]byte(payload), &task)
	if err != nil {
		return err
	}

	span := traceClient.NewSpan("/requestrender") // TODO(jbd): make /memcreate top-level span optional
	defer span.Finish()
  outputPrefix := "out."+jobIdStr
	outputBasePath := "gs://"+gcsBucketName+"/"+outputPrefix
	req := &pb.RenderRequest{
		GcsOutputBase: outputBasePath,
		ImgPath:   "gs://"+gcsBucketName+"/assets/gopher.png", // TODO: parameterize from job
		Frame:     task.Frame,
	}
	_, err =
		renderClient.RenderFrame(trace.NewContext(context.Background(), span), req)

	if err != nil {
		// TODO(jessup) Swap these out for proper logging
		fmt.Fprintf(os.Stderr, "error requesting frame - %v", err)
		return err
	}

	// delete item from gifjob_processing
	err = redisClient.LRem("gifjob_processing", 1, jobString).Err()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "deleted gifjob_%s\n", jobString)

	// increment "gifjob_"+jobIdStr+"_completed_counter"
	completedTaskCount, err := redisClient.Incr("counter_completed_gifjob_" + jobIdStr).Result()
	if err != nil {
		return err
	}
	queueLength, err := redisClient.Get("counter_queued_gifjob_" + jobIdStr).Result()
	if err != nil {
		return err
	}
	// if qeuedcounter = completedcounter, mark job as done
	queueLengthInt, _ := strconv.ParseInt(queueLength, 10, 64)
	fmt.Fprintf(os.Stdout, "job_gifjob_%s : %d of %d tasks done\n", jobIdStr, completedTaskCount, queueLengthInt)
	if completedTaskCount == queueLengthInt {
		finalImagePath, err := compileGifs(outputPrefix)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "final image path: %s\n", finalImagePath)
		var job = renderJob{
			Status:	pb.GetJobResponse_DONE,
			FinalImagePath: finalImagePath,
		}
		payloadBytes, _ := json.Marshal(job)
		err = redisClient.Set("job_gifjob_"+jobIdStr, payloadBytes, 0).Err()
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "completed job_gifjob_%s : %d tasks\n", jobIdStr, completedTaskCount)
	}

	return nil
}

/**
 * compileGifs() will glob all GCS objects prefixed with prefix, and
 * stitch them together into an animated GIF, store that in GCS and return the
 * path of the final image
 */
func compileGifs(prefix string) (string, error) {
	ctx := context.Background()
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
	    return "", err
	}
	it := gcsClient.Bucket(gcsBucketName).Objects(ctx, &storage.Query{Prefix: prefix, Versions: false})
	finalGif := &gif.GIF{}
	for {
    objAttrs, err := it.Next()
		fmt.Fprintf(os.Stdout, "DEBUG bucket %s prefix %s attrs %v\n", gcsBucketName, prefix, objAttrs)
    if err == iterator.Done {
        break
    }
    if err != nil {
        return "", err
    }
		rc, err := gcsClient.Bucket(objAttrs.Bucket).Object(objAttrs.Name).NewReader(ctx)
		if err != nil {
		    return "", err
		}
		frameImg, err := gif.Decode(rc)
		if err != nil {
			return "", err
		}
		rc.Close()
		finalGif.Image = append(finalGif.Image, frameImg.(*image.Paletted))
  	finalGif.Delay = append(finalGif.Delay, 0)
	}

  finalObjName := prefix+"/animated.gif"
	finalObj := gcsClient.Bucket(gcsBucketName).Object(finalObjName)
	wc := finalObj.NewWriter(ctx)

	wc.ObjectAttrs.ContentType = "image/gif"
	fmt.Fprintf(os.Stdout, "starting writing final: %s\n", finalObjName)
	err = gif.EncodeAll(wc, finalGif)
	if err != nil {
		return "", err
	}
	wc.Close()

	// Make the final image public
	if err := finalObj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
	    return "", err
	}

	// Return GCS URI to the public image (sans the protcol)
	return gcsBucketName+".storage.googleapis.com/"+finalObjName, nil
}

func (server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	var job renderJob
	statusStr, err := redisClient.Get("job_gifjob_" + string(req.JobId)).Result()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stdout, "status of gifjob_%s is %s\n", req.JobId, statusStr)
  err = json.Unmarshal([]byte(statusStr), &job)
	if err != nil {
		return nil, err
	}
	response := pb.GetJobResponse{ImageUrl: job.FinalImagePath, Status: job.Status}
	return &response, nil
}

func main() {
	flag.Parse()
	port := os.Getenv("GIFCREATOR_PORT")
	i, err := strconv.Atoi(port)
	if (err != nil) || (i < 1) {
		log.Fatalf("please set env var GIFCREATOR_PORT to a valid port")
	}

	// TODO(jessup) Need stricter checking here.
	redisName := os.Getenv("REDIS_NAME")
	redisPort := os.Getenv("REDIS_PORT")
	projectID := os.Getenv("GOOGLE_PROJECT_ID")
	renderName := os.Getenv("RENDER_NAME")
	renderPort := os.Getenv("RENDER_PORT")
  renderHostAddr := renderName+":"+renderPort

	gcsBucketName = os.Getenv("GCS_BUCKET_NAME")

	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisName + ":" + redisPort,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	if *workerMode == true {
		// Worker mode will perpetually poll the queue and lease tasks
		fmt.Fprintf(os.Stdout, "starting gifcreator in worker mode\n")

		ctx := context.Background()
		traceClient, err = trace.NewClient(ctx, projectID, trace.EnableGRPCTracing)
		if err != nil {
			log.Fatal(err)
		}

		conn, err := grpc.Dial(renderHostAddr,
			trace.EnableGRPCTracingDialOption, grpc.WithInsecure())

		if err != nil {
			// TODO(jessup) Swap these out for proper logging
			fmt.Fprintf(os.Stderr, "cannot connect to render service %s\n%v", renderHostAddr, err)
			return
		}
		defer conn.Close()

		renderClient = pb.NewRenderClient(conn)

		for {
			err := leaseNextTask()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error working on task: %v\n", err)
			}
			time.Sleep(10 * time.Millisecond)
			// TODO(jessup) add timed sweeps for crashed jobs that never finished processing
		}
	} else {
		// Server mode will act as a gRPC server
		fmt.Fprintf(os.Stdout, "starting gifcreator in server mode\n")
		l, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("listen failed: %v", err)
		}
		srv := grpc.NewServer()
		pb.RegisterGifCreatorServer(srv, server{})
		srv.Serve(l)
	}
}
