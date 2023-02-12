package replica

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/sourcerpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/types/known/emptypb"
)

// sourcerSrv serves a sourcer.
type sourcerSrv struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger

	mut         sync.Mutex
	srv         *gorums.Server
	awaitingTCs map[tcID]chan<- error
	tcCache     *tcCache
	hash        hash.Hash
}

// newSourcerServer returns a new sourcer server.
func newSourcerServer(conf Config, srvOpts []gorums.ServerOption) (srv *sourcerSrv) {
	srv = &sourcerSrv{
		awaitingTCs: make(map[tcID]chan<- error),
		srv:         gorums.NewServer(srvOpts...),
		tcCache:     newTcCache(),
		hash:        sha256.New(),
	}
	sourcerpb.RegisterSourcerServer(srv.srv, srv)
	return srv
}

// InitModule gives the module access to the other modules.
func (srv *sourcerSrv) InitModule(mods *modules.Core) {
	mods.Get(
		&srv.eventLoop,
		&srv.logger,
	)
	srv.tcCache.InitModule(mods)
}

func (srv *sourcerSrv) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv.StartOnListener(lis)
	return nil
}

func (srv *sourcerSrv) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

func (srv *sourcerSrv) Stop() {
	srv.srv.Stop()
}

func (srv *sourcerSrv) CollectTC(ctx gorums.ServerCtx, tc *sourcerpb.TC) (*emptypb.Empty, error) {
	id := tcID{tc.SourcerID, tc.SequenceNumber}

	c := make(chan error)
	srv.mut.Lock()
	srv.awaitingTCs[id] = c
	srv.mut.Unlock()

	srv.tcCache.addTC(tc)
	ctx.Release()
	err := <-c
	return &emptypb.Empty{}, err
}
