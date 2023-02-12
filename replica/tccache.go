package replica

import (
	"container/list"
	"context"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/sourcerpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/proto"
)

type tcCache struct {
	logger logging.Logger

	mut         sync.Mutex
	c           chan struct{}
	cache       list.List
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

func newTcCache() *tcCache {
	return &tcCache{
		c:           make(chan struct{}),
		marshaler:   proto.MarshalOptions{Deterministic: true},
		unmarshaler: proto.UnmarshalOptions{DiscardUnknown: true},
	}
}

// InitModule gives the module access to the other modules.
func (t *tcCache) InitModule(mods *modules.Core) {
	mods.Get(&t.logger)
}

func (t *tcCache) addTC(tc *sourcerpb.TC) {
	t.mut.Lock()
	defer t.mut.Unlock()
	t.cache.PushBack(tc)
	t.logger.Infof("Add %v", tc.TimedCommitment)
}

// GetTC returns all TCs to propose.
func (t *tcCache) GetTC(ctx context.Context) (tcSet hotstuff.TCSet, ok bool) {
	t.logger.Infof("enter ")
	t.mut.Lock()
	defer t.mut.Unlock()

	for i := 0; i < t.cache.Len(); i++ {
		elem := t.cache.Front()
		if elem == nil {
			break
		}
		// t.cache.Remove(elem)
		tc := elem.Value.(*sourcerpb.TC)
		tcSet[i] = tc.TimedCommitment
	}

	return tcSet, true
}
