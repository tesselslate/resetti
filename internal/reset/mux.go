package reset

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/mc"
)

// stateUpdate acts as a tuple containing a state update and the ID of the
// instance it belongs to.
type stateUpdate struct {
	State mc.InstanceState
	Id    int
}

// mux takes a slice of LogReaders as input and multiplexes their outputs onto
// one single channel, which can be selected on. Whenever an error is received
// over the returned error channel, resetti should terminate.
func mux(readers []LogReader) (<-chan stateUpdate, <-chan error) {
	ch := make(chan stateUpdate, update_channel_size*len(readers))
	errch := make(chan error, 1)
	for i, v := range readers {
		go func(i int, v LogReader) {
			for {
				select {
				case err := <-v.Errors:
					errch <- errors.Wrap(err, fmt.Sprintf("mux %d", i))
					return
				case evt, ok := <-v.Events:
					ch <- stateUpdate{evt, i}
					if !ok {
						errch <- errors.Errorf("no more updates from reader %d", i)
						return
					}
				}
			}
		}(i, v)
	}
	return ch, errch
}
