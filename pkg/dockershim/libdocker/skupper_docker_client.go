package libdocker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	//    "log"
	"regexp"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	// dockerimagetypes "github.com/docker/docker/api/types/image"
	dockernetworktypes "github.com/docker/docker/api/types/network"
	dockerapi "github.com/docker/docker/client"
	dockermessage "github.com/docker/docker/pkg/jsonmessage"
	dockerstdcopy "github.com/docker/docker/pkg/stdcopy"
)

// skupDockerClient is a wrapped layer of kocker client for skupplet internal use. This layer is added to:
//	1) Redirect stream for exec and attach operations.
//	2) Wrap the context in this layer to make the Interface cleaner.
// and is primarily derived from kubelet implementation
type skupDockerClient struct {
	// timeout is the timeout of short running docker operations.
	timeout time.Duration
	// If no pulling progress is made before imagePullProgressDeadline, the image pulling will be cancelled.
	// Docker reports image progress for every 512kB block, so normally there shouldn't be too long interval
	// between progress updates.
	imagePullProgressDeadline time.Duration
	client                    *dockerapi.Client
}

// Make sure that skupDockerClient implemented the Interface.
var _ Interface = &skupDockerClient{}

const (
	// defaultTimeout is the default timeout of short running docker operations.
	// Value is slightly offset from 2 minutes to make timeouts due to this
	// constant recognizable.
	defaultTimeout = 2*time.Minute - 1*time.Second

	// defaultShmSize is the default ShmSize to use (in bytes) if not specified.
	defaultShmSize = int64(1024 * 1024 * 64)

	// defaultImagePullingProgressReportInterval is the default interval of image pulling progress reporting.
	defaultImagePullingProgressReportInterval = 10 * time.Second
)

func newSkupDockerClient(dockerClient *dockerapi.Client, requestTimeout, imagePullProgressDeadline time.Duration) Interface {
	if requestTimeout == 0 {
		requestTimeout = defaultTimeout
	}

	skup := &skupDockerClient{
		client:                    dockerClient,
		timeout:                   requestTimeout,
		imagePullProgressDeadline: imagePullProgressDeadline,
	}

	ctx, cancel := skup.getTimeoutContext()
	defer cancel()
	dockerClient.NegotiateAPIVersion(ctx)

	return skup
}

func (d *skupDockerClient) ListContainers(options dockertypes.ContainerListOptions) ([]dockertypes.Container, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	containers, err := d.client.ContainerList(ctx, options)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func (d *skupDockerClient) InspectContainer(id string) (*dockertypes.ContainerJSON, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	containerJSON, err := d.client.ContainerInspect(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &containerJSON, nil
}

func (d *skupDockerClient) CreateContainer(opts dockertypes.ContainerCreateConfig) (*dockercontainer.ContainerCreateCreatedBody, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	if opts.HostConfig != nil && opts.HostConfig.ShmSize <= 0 {
		opts.HostConfig.ShmSize = defaultShmSize
	}
	createResp, err := d.client.ContainerCreate(ctx, opts.Config, opts.HostConfig, opts.NetworkingConfig, opts.Name)

	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &createResp, nil
}

func (d *skupDockerClient) StartContainer(id string) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (d *skupDockerClient) RestartContainer(id string, timeout time.Duration) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.ContainerRestart(ctx, id, &timeout)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (d *skupDockerClient) StopContainer(id string, timeout time.Duration) error {
	ctx, cancel := d.getCustomTimeoutContext(timeout)
	defer cancel()
	err := d.client.ContainerStop(ctx, id, &timeout)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (d *skupDockerClient) RemoveContainer(id string, opts dockertypes.ContainerRemoveOptions) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.ContainerRemove(ctx, id, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (d *skupDockerClient) UpdateContainerResources(id string, updateConfig dockercontainer.UpdateConfig) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	_, err := d.client.ContainerUpdate(ctx, id, updateConfig)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (d *skupDockerClient) inspectImageRaw(ref string) (*dockertypes.ImageInspect, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, _, err := d.client.ImageInspectWithRaw(ctx, ref)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		if dockerapi.IsErrNotFound(err) {
			err = ImageNotFoundError{ID: ref}
		}
		return nil, err
	}

	return &resp, nil
}

func (d *skupDockerClient) InspectImageByID(imageID string) (*dockertypes.ImageInspect, error) {
	resp, err := d.inspectImageRaw(imageID)
	if err != nil {
		return nil, err
	}

	//if !matchImageIDOnly(*resp, imageID) {
	//	return nil, ImageNotFoundError{ID: imageID}
	//}
	return resp, nil
}

func (d *skupDockerClient) InspectImageByRef(imageRef string) (*dockertypes.ImageInspect, error) {
	resp, err := d.inspectImageRaw(imageRef)
	if err != nil {
		return nil, err
	}

	//if !matchImageTagOrSHA(*resp, imageRef) {
	//	return nil, ImageNotFoundError{ID: imageRef}
	//}
	return resp, nil
}

func (d *skupDockerClient) ListImages(opts dockertypes.ImageListOptions) ([]dockertypes.ImageSummary, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	images, err := d.client.ImageList(ctx, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return images, nil
}

func base64EncodeAuth(auth dockertypes.AuthConfig) (string, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(auth); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes()), nil
}

// progress is a wrapper of dockermessage.JSONMessage with a lock protecting it.
type progress struct {
	sync.RWMutex
	// message stores the latest docker json message.
	message *dockermessage.JSONMessage
	// timestamp of the latest update.
	timestamp time.Time
}

func newProgress() *progress {
	return &progress{timestamp: time.Now()}
}

func (p *progress) set(msg *dockermessage.JSONMessage) {
	p.Lock()
	defer p.Unlock()
	p.message = msg
	p.timestamp = time.Now()
}

func (p *progress) get() (string, time.Time) {
	p.RLock()
	defer p.RUnlock()
	if p.message == nil {
		return "No progress", p.timestamp
	}
	// The following code is based on JSONMessage.Display
	var prefix string
	if p.message.ID != "" {
		prefix = fmt.Sprintf("%s: ", p.message.ID)
	}
	if p.message.Progress == nil {
		return fmt.Sprintf("%s%s", prefix, p.message.Status), p.timestamp
	}
	return fmt.Sprintf("%s%s %s", prefix, p.message.Status, p.message.Progress.String()), p.timestamp
}

// progressReporter keeps the newest image pulling progress and periodically report the newest progress.
type progressReporter struct {
	*progress
	image                     string
	cancel                    context.CancelFunc
	stopCh                    chan struct{}
	imagePullProgressDeadline time.Duration
}

// newProgressReporter creates a new progressReporter for specific image with specified reporting interval
func newProgressReporter(image string, cancel context.CancelFunc, imagePullProgressDeadline time.Duration) *progressReporter {
	return &progressReporter{
		progress:                  newProgress(),
		image:                     image,
		cancel:                    cancel,
		stopCh:                    make(chan struct{}),
		imagePullProgressDeadline: imagePullProgressDeadline,
	}
}

// start starts the progressReporter
func (p *progressReporter) start() {
	go func() {
		ticker := time.NewTicker(defaultImagePullingProgressReportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, timestamp := p.progress.get()
				// If there is no progress for p.imagePullProgressDeadline, cancel the operation.
				if time.Since(timestamp) > p.imagePullProgressDeadline {
					//log.Printf("Cancel pulling image %q because of no progress for %v, latest progress: %q", p.image, p.imagePullProgressDeadline, progress)
					//log.Println()
					p.cancel()
					return
				}
				//log.Printf("Pulling image %q: %q", p.image, progress)
				//log.Println()
			case <-p.stopCh:
				//progress, _ := p.progress.get()
				//log.Printf("Stop pulling image %q: %q", p.image, progress)
				//log.Println()
				return
			}
		}
	}()
}

// stop stops the progressReporter
func (p *progressReporter) stop() {
	close(p.stopCh)
}

func (d *skupDockerClient) PullImage(image string, auth dockertypes.AuthConfig, opts dockertypes.ImagePullOptions) error {
	// RegistryAuth is the base64 encoded credentials for the registry
	base64Auth, err := base64EncodeAuth(auth)
	if err != nil {
		return err
	}
	opts.RegistryAuth = base64Auth
	ctx, cancel := d.getCancelableContext()
	defer cancel()
	resp, err := d.client.ImagePull(ctx, image, opts)
	if err != nil {
		return err
	}
	defer resp.Close()
	reporter := newProgressReporter(image, cancel, d.imagePullProgressDeadline)
	reporter.start()
	defer reporter.stop()
	decoder := json.NewDecoder(resp)
	for {
		var msg dockermessage.JSONMessage
		err := decoder.Decode(&msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if msg.Error != nil {
			return msg.Error
		}
		reporter.set(&msg)
	}
	return nil
}

func (d *skupDockerClient) RemoveImage(image string, opts dockertypes.ImageRemoveOptions) ([]dockertypes.ImageDeleteResponseItem, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.ImageRemove(ctx, image, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if dockerapi.IsErrNotFound(err) {
		return nil, ImageNotFoundError{ID: image}
	}
	return resp, err
}

func (d *skupDockerClient) Logs(id string, opts dockertypes.ContainerLogsOptions, sopts StreamOptions) error {
	ctx, cancel := d.getCancelableContext()
	defer cancel()
	resp, err := d.client.ContainerLogs(ctx, id, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	defer resp.Close()
	return d.redirectResponseToOutputStream(sopts.RawTerminal, sopts.OutputStream, sopts.ErrorStream, resp)
}

func (d *skupDockerClient) Version() (*dockertypes.Version, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.ServerVersion(ctx)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *skupDockerClient) Info() (*dockertypes.Info, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.Info(ctx)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *skupDockerClient) AttachExec(id string, opts dockertypes.ExecStartCheck) (*dockertypes.HijackedResponse, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.ContainerExecAttach(ctx, id, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *skupDockerClient) CreateExec(id string, opts dockertypes.ExecConfig) (*dockertypes.IDResponse, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.ContainerExecCreate(ctx, id, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *skupDockerClient) StartExec(startExec string, opts dockertypes.ExecStartCheck, sopts StreamOptions) error {
	ctx, cancel := d.getCancelableContext()
	defer cancel()
	if opts.Detach {
		err := d.client.ContainerExecStart(ctx, startExec, opts)
		if ctxErr := contextError(ctx); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	resp, err := d.client.ContainerExecAttach(ctx, startExec, dockertypes.ExecStartCheck{
		Detach: opts.Detach,
		Tty:    opts.Tty,
	})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	defer resp.Close()

	if sopts.ExecStarted != nil {
		// Send a message to the channel indicating that the exec has started. This is needed so
		// interactive execs can handle resizing correctly - the request to resize the TTY has to happen
		// after the call to d.client.ContainerExecAttach, and because d.holdHijackedConnection below
		// blocks, we use sopts.ExecStarted to signal the caller that it's ok to resize.
		sopts.ExecStarted <- struct{}{}
	}

	return d.holdHijackedConnection(sopts.RawTerminal || opts.Tty, sopts.InputStream, sopts.OutputStream, sopts.ErrorStream, resp)
}

func (d *skupDockerClient) InspectExec(id string) (*dockertypes.ContainerExecInspect, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	resp, err := d.client.ContainerExecInspect(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *skupDockerClient) AttachToContainer(id string, opts dockertypes.ContainerAttachOptions, sopts StreamOptions) error {
	ctx, cancel := d.getCancelableContext()
	defer cancel()
	resp, err := d.client.ContainerAttach(ctx, id, opts)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	defer resp.Close()
	return d.holdHijackedConnection(sopts.RawTerminal, sopts.InputStream, sopts.OutputStream, sopts.ErrorStream, resp)
}

func (d *skupDockerClient) redirectResponseToOutputStream(tty bool, outputStream, errorStream io.Writer, resp io.Reader) error {
	if outputStream == nil {
		outputStream = ioutil.Discard
	}
	if errorStream == nil {
		errorStream = ioutil.Discard
	}
	var err error
	if tty {
		_, err = io.Copy(outputStream, resp)
	} else {
		_, err = dockerstdcopy.StdCopy(outputStream, errorStream, resp)
	}
	return err
}

func (d *skupDockerClient) holdHijackedConnection(tty bool, inputStream io.Reader, outputStream, errorStream io.Writer, resp dockertypes.HijackedResponse) error {
	receiveStdout := make(chan error)
	if outputStream != nil || errorStream != nil {
		go func() {
			receiveStdout <- d.redirectResponseToOutputStream(tty, outputStream, errorStream, resp.Reader)
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inputStream != nil {
			io.Copy(resp.Conn, inputStream)
		}
		resp.CloseWrite()
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		return err
	case <-stdinDone:
		if outputStream != nil || errorStream != nil {
			return <-receiveStdout
		}
	}
	return nil
}

// getCancelableContext returns a new cancelable context. For long running requests without timeout, we use cancelable
// context to avoid potential resource leak, although the current implementation shouldn't leak resource.
func (d *skupDockerClient) getCancelableContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// getTimeoutContext returns a new context with default request timeout
func (d *skupDockerClient) getTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d.timeout)
}

// getCustomTimeoutContext returns a new context with a specific request timeout
func (d *skupDockerClient) getCustomTimeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	// Pick the larger of the two
	if d.timeout > timeout {
		timeout = d.timeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// contextError checks the context, and returns error if the context is timeout.
func contextError(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return operationTimeout{err: ctx.Err()}
	}
	return ctx.Err()
}

// StreamOptions are the options used to configure the stream redirection
type StreamOptions struct {
	RawTerminal  bool
	InputStream  io.Reader
	OutputStream io.Writer
	ErrorStream  io.Writer
	ExecStarted  chan struct{}
}

// operationTimeout is the error returned when the docker operations are timeout.
type operationTimeout struct {
	err error
}

func (e operationTimeout) Error() string {
	return fmt.Sprintf("operation timeout: %v", e.err)
}

// containerNotFoundErrorRegx is the regexp of container not found error message.
var containerNotFoundErrorRegx = regexp.MustCompile(`No such container: [0-9a-z]+`)

// IsContainerNotFoundError checks whether the error is container not found error.
func IsContainerNotFoundError(err error) bool {
	return containerNotFoundErrorRegx.MatchString(err.Error())
}

// ImageNotFoundError is the error returned by InspectImage when image not found.
// Expose this to inject error in dockershim for testing.
type ImageNotFoundError struct {
	ID string
}

func (e ImageNotFoundError) Error() string {
	return fmt.Sprintf("no such image: %q", e.ID)
}

// IsImageNotFoundError checks whether the error is image not found error. This is exposed
// to share with dockershim.
func IsImageNotFoundError(err error) bool {
	_, ok := err.(ImageNotFoundError)
	return ok
}

func (d *skupDockerClient) InspectNetwork(id string) (dockertypes.NetworkResource, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	nr, err := d.client.NetworkInspect(ctx, id, dockertypes.NetworkInspectOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nr, ctxErr
	}
	if err != nil {
		return nr, err
	}
	return nr, nil
}

func (d *skupDockerClient) CreateNetwork(id string) (dockertypes.NetworkCreateResponse, error) {
	// TODO: what if network exists, error? inspect first?
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	ncr, err := d.client.NetworkCreate(ctx, id, dockertypes.NetworkCreate{
		CheckDuplicate: true,
		Driver:         "bridge",
	})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ncr, ctxErr
	}
	if err != nil {
		return ncr, err
	}
	return ncr, nil
}

func (d *skupDockerClient) ConnectContainerToNetwork(id string, containerid string) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.NetworkConnect(ctx, id, containerid, &dockernetworktypes.EndpointSettings{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (d *skupDockerClient) DisconnectContainerFromNetwork(id string, containerid string, force bool) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.NetworkDisconnect(ctx, id, containerid, force)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (d *skupDockerClient) RemoveNetwork(id string) error {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	err := d.client.NetworkRemove(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}
