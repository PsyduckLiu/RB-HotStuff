// Package sourcer implements a simple sourcer for testing HotStuff.
// The sourcer reads data from an input stream and sends the data in TC to all HotStuff replicas.
package sourcer

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"math/big"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/sourcerpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type qspec struct {
	faulty int
}

func (q *qspec) CollectTCQF(_ *sourcerpb.TC, replies map[uint32]*emptypb.Empty) (*emptypb.Empty, bool) {
	return &emptypb.Empty{}, true
}

type pendingTC struct {
	sequenceNumber uint64
	sendTime       time.Time
	promise        *sourcerpb.AsyncEmpty
	cancelCtx      context.CancelFunc
}

// Config contains config options for a sourcer.
type Config struct {
	Input          io.ReadCloser
	ManagerOptions []gorums.ManagerOption
	Timeout        time.Duration
}

// sourcer is a hotstuff sourcer.
type Sourcer struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	opts      *modules.Options

	mgr          *sourcerpb.Manager
	gorumsConfig *sourcerpb.Configuration
	cancel       context.CancelFunc
	done         chan struct{}
	reader       io.ReadCloser
	timeout      time.Duration
	pendingTCs   chan pendingTC
}

// InitModule initializes the sourcer.
func (s *Sourcer) InitModule(mods *modules.Core) {
	mods.Get(
		&s.eventLoop,
		&s.logger,
		&s.opts,
	)
}

// New returns a new Sourcer.
func New(conf Config, builder modules.Builder) (sourcer *Sourcer) {
	sourcer = &Sourcer{
		pendingTCs: make(chan pendingTC, 1000),
		done:       make(chan struct{}),
		reader:     conf.Input,
		timeout:    conf.Timeout,
	}

	builder.Add(sourcer)
	builder.Build()

	creds := insecure.NewCredentials()
	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
	grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))

	opts := conf.ManagerOptions
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	sourcer.mgr = sourcerpb.NewManager(opts...)

	return sourcer
}

// Connect connects the sourcer to the replicas.
func (s *Sourcer) Connect(replicas []backend.ReplicaInfo) (err error) {
	nodes := make(map[string]uint32, len(replicas))
	for _, r := range replicas {
		nodes[r.Address] = uint32(r.ID)
	}
	s.gorumsConfig, err = s.mgr.NewConfiguration(&qspec{faulty: hotstuff.NumFaulty(len(replicas))}, gorums.WithNodeMap(nodes))
	// s.gorumsConfig, err = s.mgr.NewConfiguration(gorums.WithNodeMap(nodes))
	if err != nil {
		s.mgr.Close()
		return err
	}
	return nil
}

// Run runs the sourcer until the context is closed.
func (s *Sourcer) Run(ctx context.Context) {
	type stats struct {
		executed int
		failed   int
		timeout  int
	}

	eventLoopDone := make(chan struct{})
	go func() {
		s.eventLoop.Run(ctx)
		close(eventLoopDone)
	}()
	s.logger.Info("SOURCER Starting to send commands")

	commandStatsChan := make(chan stats)
	// start the command handler
	go func() {
		executed, failed, timeout := s.handleCommands(ctx)
		commandStatsChan <- stats{executed, failed, timeout}
	}()

	err := s.sendTCs(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		s.logger.Panicf("Failed to send commands: %v", err)
	}
	s.close()

	commandStats := <-commandStatsChan
	s.logger.Infof(
		"Done sending commands (executed: %d, failed: %d, timeouts: %d)",
		commandStats.executed, commandStats.failed, commandStats.timeout,
	)
	<-eventLoopDone
	close(s.done)
}

// Start starts the sourcer.
func (s *Sourcer) Start() {
	var ctx context.Context
	ctx, s.cancel = context.WithCancel(context.Background())
	go s.Run(ctx)
}

// Stop stops the sourcer.
func (s *Sourcer) Stop() {
	s.cancel()
	<-s.done
}

func (s *Sourcer) close() {
	s.mgr.Close()
	err := s.reader.Close()
	if err != nil {
		s.logger.Warn("Failed to close reader: ", err)
	}
}

func (s *Sourcer) sendTCs(ctx context.Context) error {
	var num uint64 = 1

loop:
	for {
		if ctx.Err() != nil {
			break
		}

		randData, _ := rand.Int(rand.Reader, big.NewInt(1000000))
		data := randData.Bytes()
		// s.logger.Infof("Random data %v is %v", randData, data)

		tc := &sourcerpb.TC{
			SourcerID:       uint32(s.opts.ID()),
			SequenceNumber:  num,
			TimedCommitment: data,
		}
		// s.logger.Infof("TC data from %d is %v", s.opts.ID(), data)

		ctx, cancel := context.WithTimeout(ctx, s.timeout)
		promise := s.gorumsConfig.CollectTC(ctx, tc)
		pending := pendingTC{sequenceNumber: num, sendTime: time.Now(), promise: promise, cancelCtx: cancel}

		num++
		select {
		case s.pendingTCs <- pending:
		case <-ctx.Done():
			break loop
		}

		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// handleTCs will get pending commands from the pendingCmds channel and then
// handle them as they become acknowledged by the replicas. We expect the commands to be
// acknowledged in the order that they were sent.
func (s *Sourcer) handleCommands(ctx context.Context) (executed, failed, timeout int) {
	for {
		var (
			tc pendingTC
			ok bool
		)
		select {
		case tc, ok = <-s.pendingTCs:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
		_, err := tc.promise.Get()
		if err != nil {
			qcError, ok := err.(gorums.QuorumCallError)
			if ok && qcError.Reason == context.DeadlineExceeded.Error() {
				s.logger.Debug("Command timed out.")
				timeout++
			} else if !ok || qcError.Reason != context.Canceled.Error() {
				s.logger.Debugf("Did not get enough replies for command: %v\n", err)
				failed++
			}
		} else {
			executed++
		}

	}
}
