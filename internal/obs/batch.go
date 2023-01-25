package obs

// Batch contains multiple requests which are to be submitted and executed
// at once.
type Batch struct {
	requests []request
	client   *Client
}

// BatchMode contains the mode the requests of a batch are to be executed in.
type BatchMode int

// Request batch modes.
const (
	SerialRealtime BatchMode = iota
	SerialFrame
	Parallel
)

// newBatch creates a new batch.
func newBatch(client *Client) Batch {
	return Batch{
		requests: make([]request, 0),
		client:   client,
	}
}

func (b *Batch) SetItemPosition(scene, name string, x, y, w, h float64) {
	id, err := b.client.getSceneItemId(scene, name)
	if err != nil {
		panic(err)
	}
	req := reqSetSceneItemTransform(scene, id, x, y, w, h)
	b.requests = append(b.requests, req)
}

func (b *Batch) SetItemVisibility(scene, name string, visible bool) {
	id, err := b.client.getSceneItemId(scene, name)
	if err != nil {
		panic(err)
	}
	req := reqSetSceneItemVisible(scene, id, visible)
	b.requests = append(b.requests, req)
}

func (b *Batch) SetSourceSettings(source string, settings StringMap, keep bool) {
	req := reqSetSourceSettings(source, settings, keep)
	b.requests = append(b.requests, req)
}
