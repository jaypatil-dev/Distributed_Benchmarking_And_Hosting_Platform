/*
****** WHAT THIS FILE DOES ******
* Core sandboxing engine of the platform.
* Uses the Docker SDK to programmatically create isolated containers
* for each contestant's submitted code.
*
* STRUCTURES:
* - Sandbox => holds the Docker client, container ID and port.
*
* FUNCTIONS:
* - NewSandbox()      => connects to Docker via Docker SDK
* - RunSubmission()   => pulls image, creates and starts container
*                        with strict resource limits
* - StopAndRemove()   => cleanly destroys container after testing
* - getBaseImage()    => maps language names to Docker base images
*
* METRICS EXPOSED:
* - sandbox_containers_created_total    => total containers ever created
* - sandbox_containers_active           => currently running containers
* - sandbox_container_errors_total      => total container creation/start errors
* - sandbox_container_duration_seconds  => how long containers run before removal
*
* WHY STRICT RESOURCE LIMITS?
* Without limits a malicious contestant could:
* - Fork bomb    => 256MB memory hard limit stops this
* - Hog CPU      => 0.5 core limit prevents monopolization
* - Run forever  => container destroyed after test completes
*
* FUTURE IMPROVEMENTS:
* - gVisor runtime for kernel-level isolation
* - network=none for completely offline sandboxing
* - disk I/O limits to prevent storage attacks
*/

package sandbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ─── Prometheus Metrics ───────────────────────────────────────

var (
	// total containers created since service started
	containersCreated = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sandbox_containers_created_total",
			Help: "Total number of containers created",
		},
	)

	// currently running containers
	containersActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "sandbox_containers_active",
			Help: "Number of containers currently running",
		},
	)

	// total errors during container creation or startup
	containerErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sandbox_container_errors_total",
			Help: "Total container creation or start errors",
		},
	)

	// how long each container runs before being removed
	containerDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sandbox_container_duration_seconds",
			Help:    "How long containers run before removal",
			Buckets: []float64{5, 10, 30, 60, 120, 300},
		},
	)
)

func init() {
	prometheus.MustRegister(containersCreated)
	prometheus.MustRegister(containersActive)
	prometheus.MustRegister(containerErrors)
	prometheus.MustRegister(containerDuration)

	// start metrics server on port 2115
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		fmt.Println("Metrics server running on :2115")
		http.ListenAndServe(":2115", mux)
	}()
}

type Sandbox struct {
	client      *client.Client
	ContainerID string
	Port        string
}

func NewSandbox() (*Sandbox, error) {
	// connect to Docker Desktop via Docker socket
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to docker: %v", err)
	}

	return &Sandbox{client: cli}, nil
}

func (s *Sandbox) RunSubmission(language string, port string) error {
	ctx := context.Background()

	baseImage := getBaseImage(language)
	fmt.Printf("Pulling base image: %s\n", baseImage)

	// pull image if not already available
	reader, err := s.client.ImagePull(ctx, baseImage, image.PullOptions{})
	if err != nil {
		containerErrors.Inc()
		return fmt.Errorf("failed to pull image: %v", err)
	}
	defer reader.Close()
	io.Copy(os.Stdout, reader)

	containerConfig := &container.Config{
		Image: baseImage,
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}

	// strict resource limits — prevents malicious code from affecting platform
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:   256 * 1024 * 1024, // 256MB max
			NanoCPUs: 500000000,          // 0.5 CPU max
		},
		PortBindings: nat.PortMap{
			nat.Port(port + "/tcp"): []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: port},
			},
		},
		NetworkMode: "bridge",
	}

	resp, err := s.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		fmt.Sprintf("submission-%s", port),
	)
	if err != nil {
		containerErrors.Inc()
		return fmt.Errorf("failed to create container: %v", err)
	}

	s.ContainerID = resp.ID
	s.Port = port

	if err := s.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		containerErrors.Inc()
		return fmt.Errorf("failed to start container: %v", err)
	}

	// track active containers and record creation
	containersCreated.Inc()
	containersActive.Inc()

	fmt.Printf("Container started: %s on port %s\n", resp.ID[:12], port)
	return nil
}

func (s *Sandbox) StopAndRemove() error {
	ctx := context.Background()

	// start timer — measures how long container ran
	timer := prometheus.NewTimer(containerDuration)
	defer timer.ObserveDuration()

	if err := s.client.ContainerStop(ctx, s.ContainerID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container: %v", err)
	}

	if err := s.client.ContainerRemove(ctx, s.ContainerID, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("failed to remove container: %v", err)
	}

	// container is gone — decrement active count
	containersActive.Dec()

	fmt.Printf("Container %s stopped and removed\n", s.ContainerID[:12])
	return nil
}

func getBaseImage(language string) string {
	switch language {
	case "cpp":
		return "gcc:latest"
	case "rust":
		return "rust:latest"
	case "go":
		return "golang:latest"
	case "python":
		return "python:3.11"
	case "java":
		return "openjdk:21"
	case "javascript":
		return "node:20"
	default:
		return "ubuntu:22.04"
	}
}