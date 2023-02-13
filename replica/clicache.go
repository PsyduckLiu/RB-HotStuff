package replica

import (
	"container/list"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/sourcerpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/proto"
)

type cliCache struct {
	logger logging.Logger

	mut         sync.Mutex
	c           chan struct{}
	cache       list.List
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

func newCliCache() *cliCache {
	return &cliCache{
		c:           make(chan struct{}),
		marshaler:   proto.MarshalOptions{Deterministic: true},
		unmarshaler: proto.UnmarshalOptions{DiscardUnknown: true},
	}
}

// InitModule gives the module access to the other modules.
func (c *cliCache) InitModule(mods *modules.Core) {
	mods.Get(&c.logger)
}

func (c *cliCache) AddTCSet(tcSet hotstuff.TCSet) {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.cache.PushBack(tcSet)
	c.logger.Infof("Add %v", tcSet)
}

// GetTC returns all TCs to propose.
func (c *cliCache) GetTCSet() (tcSet hotstuff.TCSet, ok bool) {
	c.logger.Infof("enter ")
	c.mut.Lock()
	defer c.mut.Unlock()

	result := make(hotstuff.TCSet)
	for i := 0; i < c.cache.Len(); i++ {
		elem := c.cache.Front()
		if elem == nil {
			break
		}
		c.cache.Remove(elem)
		tc := elem.Value.(*sourcerpb.TC)
		result[i] = tc.TimedCommitment
	}

	return result, true
}
