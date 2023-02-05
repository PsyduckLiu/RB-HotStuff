package metrics

import (
	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
)

func init() {
	RegisterReplicaMetric("newOutput", func() any {
		return &NewOutput{}
	})
	RegisterClientMetric("newOutput", func() any {
		return &NewOutput{}
	})
}

// NewOutput gets new committed output per second.
type NewOutput struct {
	metricsLogger Logger
	opts          *modules.Options
	logger        logging.Logger

	newId     []hotstuff.ID
	newOutput []string
}

// InitModule gives the module access to the other modules.
func (no *NewOutput) InitModule(mods *modules.Core) {
	var (
		eventLoop *eventloop.EventLoop
		logger    logging.Logger
	)

	mods.Get(
		&no.metricsLogger,
		&no.opts,
		&no.logger,
		&eventLoop,
		&logger,
	)

	eventLoop.RegisterHandler(hotstuff.OutputEvent{}, func(event any) {
		outputEvent := event.(hotstuff.OutputEvent)
		no.recordOutput(outputEvent.Output, outputEvent.Id)
	})

	eventLoop.RegisterObserver(types.TickEvent{}, func(event any) {
		no.tick(event.(types.TickEvent))
	})

	logger.Info("NewOutput metric enabled")
}

func (no *NewOutput) recordOutput(output string, id hotstuff.ID) {
	no.newId = append(no.newId, id)
	no.newOutput = append(no.newOutput, output)
}

func (no *NewOutput) tick(tick types.TickEvent) {
	for index, output := range no.newOutput {
		proposalID := no.newId[index]
		// no.logger.Infof("ID is %v", no.opts.ID())
		// no.logger.Infof("proposalID is %v", proposalID)
		if proposalID == no.opts.ID() {
			no.logger.Infof("New output is %v", []byte(output))
		}
	}

	no.newId = []hotstuff.ID{}
	no.newOutput = []string{}
	// reset count for next tick
	// t.commandCount = 0
	// t.commitCount = 0
}
