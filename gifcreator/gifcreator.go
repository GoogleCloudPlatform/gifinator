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

	"gopkg.in/redis.v5"

	pb "github.com/GoogleCloudPlatform/k8s-render-demo/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"cloud.google.com/go/trace"
)

type server struct{}

var (
	redisClient  *redis.Client
	renderClient pb.RenderClient
	workerMode   = flag.Bool("worker", false, "run in worker mode rather than server")
	traceClient  *trace.Client
)

func (server) StartJob(ctx context.Context, req *pb.StartJobRequest) (*pb.StartJobResponse, error) {
	// TODO(jessup) this should be stored as a job in Redis

	// Retrieive the next job ID from Redis
	jobId, err := redisClient.Incr("gifjob_counter").Result()
	if err != nil {
		return nil, err
	}
	jobIdStr := strconv.FormatInt(jobId, 10)
	// Create a new RenderJob queue for that job
	err = redisClient.Set("job_gifjob_"+jobIdStr, "PENDING", 0).Err()
	if err != nil {
		return nil, err
	}

	// Add tasks to the GifJob queue for each frame to render
	var taskId int64
	var taskIdStr string
	for i := 0; i < 36; i++ {
		//Get new task id
		taskId, err = redisClient.Incr("counter_queued_gifjob_" + jobIdStr).Result()
		if err != nil {
			return nil, err
		}
		taskIdStr = strconv.FormatInt(taskId, 10)
		//SET gifjob_JOBID:TASK_ID with serialized pb.RenderRequest
		err = redisClient.Set("task_gifjob_"+jobIdStr+"_"+taskIdStr, "will_be_serialized", 0).Err()
		if err != nil {
			return nil, err
		}
		//LPUSH "gifjob_JOBID_queued" "TASK_ID"
		err = redisClient.LPush("gifjob_queued", jobIdStr+"_"+taskIdStr).Err()
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stdout, "enqueued gifjob_%s_%s\n", jobIdStr, taskIdStr)
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
	taskId, _ := strconv.ParseInt(taskIdStr, 10, 64)

	payload, err := redisClient.Get("task_gifjob_" + jobString).Result()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "leased gifjob_%s %s\n", jobString, payload)

	// DO WORK

	// TODO(jessup) Actually de-serialize the payload and translate into
	// tasks for the API.
	span := traceClient.NewSpan("/requestrender") // TODO(jbd): make /memcreate top-level span optional
	defer span.Finish()
	req := &pb.RenderRequest{
		GcsOutputBase: os.TempDir(),
		ImgPath:   "/render/gopher.png", // TODO: something real
		Frame:     taskId,
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
		err = redisClient.Set("job_gifjob_"+jobIdStr, "DONE", 0).Err()
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "completed job_gifjob_%s : %d tasks\n", jobIdStr, completedTaskCount)
	}

	return nil
}

func (server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	// TODO(jessup) look this up from a Reids service
	var status pb.GetJobResponse_Status
	statusStr, err := redisClient.Get("job_gifjob_" + string(req.JobId)).Result()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stdout, "status of gifjob_%s is %s\n", req.JobId, statusStr)
	switch statusStr {
	case "PENDING":
		status = pb.GetJobResponse_PENDING
	case "DONE":
		status = pb.GetJobResponse_DONE
	default:
		status = pb.GetJobResponse_UNKNOWN_STATUS
	}
	response := pb.GetJobResponse{ImageUrl: "", Status: status}
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
